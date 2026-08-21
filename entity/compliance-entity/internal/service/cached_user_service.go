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
	"time"

	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/cache"
	"github.com/wso2-open-operations/grc-tools/entity/compliance-entity/internal/domain"
)

type cachedUserService struct {
	inner  UserService
	byID   *cache.Cache[int, domain.User]
	byUUID *cache.Cache[string, domain.User]
}

// NewCachedUserService wraps inner with a 5-minute in-memory cache on
// GetUserByID and GetUserByUUID. Entries are invalidated when CreateUser or
// UpdateUser is called.
func NewCachedUserService(inner UserService) UserService {
	ttl := 5 * time.Minute
	return &cachedUserService{
		inner:  inner,
		byID:   cache.New[int, domain.User](ttl),
		byUUID: cache.New[string, domain.User](ttl),
	}
}

// remember caches a user under every key that identifies it, so a lookup by one
// key warms the other. A legacy row with no UUID is only cached by ID, so it
// can never be served back out under the empty-UUID key.
func (s *cachedUserService) remember(u domain.User) {
	s.byID.Set(u.ID, u)
	if u.UUID != "" {
		s.byUUID.Set(u.UUID, u)
	}
}

// forget evicts a user from every key. Called on writes, including the ones
// that only *might* have changed the row.
func (s *cachedUserService) forget(u domain.User) {
	s.byID.Delete(u.ID)
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

// CreateUser is an upsert, so it can modify an existing row. Evicting
// afterwards is therefore required, not defensive: without it, a resolve that
// has just refreshed a user's row would keep serving the pre-write one for
// the rest of the TTL.
func (s *cachedUserService) CreateUser(ctx context.Context, req domain.CreateUserRequest) (domain.User, error) {
	if old, ok := s.byUUID.Get(req.UUID); ok {
		s.forget(old)
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
