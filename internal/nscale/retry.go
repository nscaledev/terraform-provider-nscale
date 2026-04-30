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
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// IsAPIErrorInUse reports whether err is the Nscale API's "resource is in
// use" rejection (HTTP 403 with an "in use" message). Use this to decide
// whether a delete is worth retrying — see RetryDelete.
func IsAPIErrorInUse(err error) bool {
	e, ok := AsAPIError(err)
	if !ok {
		return false
	}
	return e.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(e.Message), "in use")
}

// RetryDelete invokes deleteFn until it succeeds or the timeout elapses, and
// is the nscale equivalent of the retry-on-DependencyViolation pattern that
// terraform-provider-aws uses for aws_security_group.
//
// It exists because Terraform core does not order update(parent) → delete(child)
// when the parent is being updated in the same plan to drop its reference to
// the child being deleted (hashicorp/terraform#32136, closed working-as-designed).
// Terraform dispatches the destroy in parallel with the parent update, the API
// rejects the destroy because the child is still in use, and the apply fails.
// Retrying the delete here lets the parent update land first.
//
// Return values from deleteFn:
//   - (nil, _)            → success
//   - (404 APIError, _)   → success (resource already gone)
//   - (err, true)         → retry
//   - (err, false)        → fail immediately
//
// Typical use: pass the resource's Terraform delete timeout and have deleteFn
// return retry=true for IsAPIErrorInUse(err).
func RetryDelete(ctx context.Context, timeout time.Duration, deleteFn func(context.Context) (error, bool)) error {
	return retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		err, retryable := deleteFn(ctx)
		if err == nil {
			return nil
		}
		if e, ok := AsAPIError(err); ok && e.StatusCode == http.StatusNotFound {
			return nil
		}
		if retryable {
			return retry.RetryableError(err)
		}
		return retry.NonRetryableError(err)
	})
}
