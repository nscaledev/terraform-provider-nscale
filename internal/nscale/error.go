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
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	TraceID    *string

	// The following fields are set only when the error is created while parsing an API response body.
	Endpoint  string
	BodyBytes []byte
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

	if e.TraceID != nil && *e.TraceID != "" {
		builder.WriteString(", trace_id: ")
		builder.WriteString(*e.TraceID)
	}

	return builder.String()
}

func AsAPIError(err error) (*APIError, bool) {
	if e := (*APIError)(nil); errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func TerraformDebugLogAPIResponseBody(ctx context.Context, err error) {
	if e, ok := AsAPIError(err); ok && len(e.BodyBytes) > 0 {
		message := "API response could not be parsed"
		if e.Endpoint != "" {
			message = fmt.Sprintf("%s response could not be parsed", e.Endpoint)
		}

		tflog.Debug(ctx, message, map[string]any{
			"response_body": string(e.BodyBytes),
		})
	}
}
