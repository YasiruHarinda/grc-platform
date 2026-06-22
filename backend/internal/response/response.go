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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package response

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/grc-platform/backend/internal/apierror"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

const (
	ErrMsgUnauthorized = "You are not authorized to perform this action."
	ErrMsgForbidden    = "Access to the requested resource is forbidden."
	ErrMsgNotFound     = "The requested resource was not found."
	ErrMsgBadRequest   = "Invalid request payload."
	ErrMsgTooLarge     = "Request body too large."
	ErrMsgInternal     = "An internal server error occurred. Please try again later."
	errMsgReadBody     = "Failed to read request body."
)

// ErrorBody is the standard JSON error envelope returned on all non-2xx responses.
type ErrorBody struct {
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(ErrorBody{Message: message})
}

func WriteJSON(w http.ResponseWriter, statusCode int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(data)
}

func WriteJSONValue(w http.ResponseWriter, statusCode int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrMsgInternal)
		return
	}
	WriteJSON(w, statusCode, data)
}

// DecodeJSON reads and unmarshals the JSON request body into v.
// On failure it writes an appropriate error response and returns a non-nil error.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			WriteError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
		} else {
			WriteError(w, http.StatusBadRequest, errMsgReadBody)
		}
		return err
	}
	if !json.Valid(body) {
		WriteError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return fmt.Errorf("invalid json")
	}
	if err := json.Unmarshal(body, v); err != nil {
		WriteError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return err
	}
	return nil
}

// MapServiceError translates a repository or service error to an HTTP response.
// ctx is used to attach the correlation ID to any logged errors.
func MapServiceError(ctx context.Context, w http.ResponseWriter, err error, fallbackMsg string) {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusNotFound:
			WriteError(w, http.StatusNotFound, ErrMsgNotFound)
		case http.StatusForbidden:
			WriteError(w, http.StatusForbidden, ErrMsgForbidden)
		case http.StatusBadRequest:
			WriteError(w, http.StatusBadRequest, ErrMsgBadRequest)
		case http.StatusConflict, http.StatusUnprocessableEntity:
			WriteError(w, apiErr.StatusCode, apiErr.Body)
		default:
			slog.ErrorContext(ctx, "unhandled service error", "err", err)
			WriteError(w, http.StatusInternalServerError, fallbackMsg)
		}
		return
	}
	slog.ErrorContext(ctx, "unhandled service error", "err", err)
	WriteError(w, http.StatusInternalServerError, fallbackMsg)
}
