/*
Copyright 2026 Nscale

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

package kubernetescluster

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestDefaultTimeoutsMatchDocs pins the timeout defaults to the published
// resource docs, which have drifted from the code before.
func TestDefaultTimeoutsMatchDocs(t *testing.T) {
	t.Parallel()

	docs, err := os.ReadFile("../../../website/docs/r/kubernetes_cluster.html.markdown")
	if err != nil {
		t.Fatalf("reading resource docs: %s", err)
	}

	tests := map[string]time.Duration{
		"create": defaultCreateTimeout,
		"update": defaultUpdateTimeout,
		"delete": defaultDeleteTimeout,
	}

	for operation, timeout := range tests {
		want := fmt.Sprintf("* `%s` - (Default `%.0fm`)", operation, timeout.Minutes())
		if !strings.Contains(string(docs), want) {
			t.Errorf("resource docs are missing %q; update the Timeouts section to match the code", want)
		}
	}
}
