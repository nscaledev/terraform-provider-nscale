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
	"fmt"
)

var ErrEmptyResponse = errors.New("server returned an empty response")

type StatusCodeError struct {
	Code int
}

func NewStatusCodeError(code int) StatusCodeError {
	return StatusCodeError{Code: code}
}

func (e StatusCodeError) Error() string {
	return fmt.Sprintf("server returned status code %d", e.Code)
}

func IsStatusCodeError(err error, code int) bool {
	if e := (StatusCodeError{}); errors.As(err, &e) {
		return e.Code == code
	}
	return false
}
