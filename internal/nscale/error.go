/*
Copyright 2025 Nscale

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nscale

import (
	"errors"
	"strconv"
	"strings"
)

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	var builder strings.Builder

	builder.WriteString("server returned status code ")
	builder.WriteString(strconv.Itoa(e.StatusCode))

	if e.Code != "" {
		builder.WriteString(", code: ")
		builder.WriteString(e.Code)
	}

	if e.Message != "" {
		builder.WriteString(", message: ")
		builder.WriteString(e.Message)
	}

	return builder.String()
}

type APIErrorMatcher func(*APIError) bool

func APIStatusCode(code int) APIErrorMatcher {
	return func(e *APIError) bool {
		return e.StatusCode == code
	}
}

func IsAPIError(err error, matchers ...APIErrorMatcher) bool {
	if e := (*APIError)(nil); errors.As(err, &e) {
		for _, matcher := range matchers {
			if !matcher(e) {
				return false
			}
		}
		return true
	}
	return false
}
