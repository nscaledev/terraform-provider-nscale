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

package version

// ProviderVersion is reported to Terraform via the provider's Metadata
// response and sent in the API user agent.
//
// release-please rewrites the literal below on every release PR, so it tracks
// the released version rather than being injected at link time — GoReleaser's
// ldflags target `main.version`, which this provider's main package does not
// define. Keep the `x-release-please-version` annotation on the same line: it
// is the anchor release-please matches, and the `extra-files` entry in
// release-please-config.json is what brings this file into scope.
var ProviderVersion = "1.5.0" // x-release-please-version
