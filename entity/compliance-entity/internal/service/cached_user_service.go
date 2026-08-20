// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"strings"
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/cache"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

// emailCacheKey normalizes an email for use as a byEmail cache key, so a
// lookup and a later invalidation always agree on the key regardless of how
// the caller or the stored row happens to case/space the address. Without
// this, a caller spelling was cached as a second, unrelated key (see
// GetUserByEmail's old comment) that forget() had no way to find and evict,
// so an alternate spelling could keep serving a stale (e.g. pre-uuid-backfill)
// row for the rest of its TTL after a write.
func emailCacheKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

type cachedUserService struct {
	inner   UserService
	byID    *cache.Cache[int, domain.User]
	byEmail *cache.Cache[string, domain.User]
	byUUID  *cache.Cache[string, domain.User]
}

// NewCachedUserService wraps inner with a 5-minute in-memory cache on
// GetUserByID, GetUserByEmail and GetUserByUUID. Entries are invalidated when
// CreateUser or UpdateUser is called.
func NewCachedUserService(inner UserService) UserService {
	ttl := 5 * time.Minute
	return &cachedUserService{
		inner:   inner,
		byID:    cache.New[int, domain.User](ttl),
		byEmail: cache.New[string, domain.User](ttl),
		byUUID:  cache.New[string, domain.User](ttl),
	}
}

// remember caches a user under every key that identifies it, so a lookup by one
// key warms the others.
//
// Both the email and uuid keys are skipped when empty: a row that predates
// the identity migration has no uuid, and an Admin-Console-provisioned row has
// no email (see nullableEmail in user_repo.go) — caching either under ""
// would make the next empty lookup return whichever user was cached last,
// and (for CreateUser's pre-write eviction below) could evict an unrelated
// user's cache entry.
func (s *cachedUserService) remember(u domain.User) {
	s.byID.Set(u.ID, u)
	if u.Email != "" {
		s.byEmail.Set(emailCacheKey(u.Email), u)
	}
	if u.UUID != "" {
		s.byUUID.Set(u.UUID, u)
	}
}

// forget evicts a user from every key. Called on writes, including the ones
// that only *might* have changed the row.
func (s *cachedUserService) forget(u domain.User) {
	s.byID.Delete(u.ID)
	if u.Email != "" {
		s.byEmail.Delete(emailCacheKey(u.Email))
	}
	if u.UUID != "" {
		s.byUUID.Delete(u.UUID)
	}
}

func (s *cachedUserService) SearchUsers(ctx context.Context, req domain.SearchUsersRequest) (domain.SearchUsersResponse, error) {
	return s.inner.SearchUsers(ctx, req)
}

func (s *cachedUserService) GetUserByID(ctx context.Context, id int) (domain.User, error) {
	if v, ok := s.byID.Get(id); ok {
		return v, nil
	}
	user, err := s.inner.GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	s.remember(user)
	return user, nil
}

func (s *cachedUserService) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	if v, ok := s.byEmail.Get(emailCacheKey(email)); ok {
		return v, nil
	}
	user, err := s.inner.GetUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, err
	}
	s.remember(user)
	return user, nil
}

func (s *cachedUserService) GetUserByUUID(ctx context.Context, uuid string) (domain.User, error) {
	if v, ok := s.byUUID.Get(uuid); ok {
		return v, nil
	}
	user, err := s.inner.GetUserByUUID(ctx, uuid)
	if err != nil {
		return domain.User{}, err
	}
	s.remember(user)
	return user, nil
}

// CreateUser is an upsert, so it can modify an existing row — it fills in a
// uuid the row was missing and refreshes display_name. Evicting afterwards is
// therefore required, not defensive: without it, a resolve that has just given
// a user their uuid would keep serving the pre-write row, with an empty uuid,
// for the rest of the TTL.
//
// Both the pre- and post-write identities are evicted. They can differ — the
// row may have been cached before it had a uuid — and only evicting the new one
// would leave the stale entry reachable by its old key.
func (s *cachedUserService) CreateUser(ctx context.Context, req domain.CreateUserRequest) (domain.User, error) {
	if req.Email != "" {
		if old, ok := s.byEmail.Get(emailCacheKey(req.Email)); ok {
			s.forget(old)
		}
	}

	user, err := s.inner.CreateUser(ctx, req)
	if err != nil {
		return domain.User{}, err
	}
	s.forget(user)
	return user, nil
}

func (s *cachedUserService) UpdateUser(ctx context.Context, id int, req domain.UpdateUserRequest) (domain.User, error) {
	// Capture the old identity before the update so its cache keys can be
	// invalidated even if the update changed them.
	old, hasOld := s.byID.Get(id)

	user, err := s.inner.UpdateUser(ctx, id, req)
	if err != nil {
		return domain.User{}, err
	}

	if hasOld {
		s.forget(old)
	}
	// Also evict the new identity in case it was cached from a previous lookup.
	s.forget(user)

	return user, nil
}
