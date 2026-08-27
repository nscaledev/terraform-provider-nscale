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

package nks

// This package is the Go client for the Nscale Kubernetes Service (NKS) API.
// The package doc proper lives in the generated nks.gen.go.
//
// Unlike every other service client in this provider, NKS is generated here
// rather than consumed from github.com/nscaledev/nscale-sdk-go. Two reasons:
//
//  1. The SDK release that carries the NKS spec (v0.2.0) also deletes the
//     nscale-sdk-go/common package and renames the identity/region operations.
//     Thirty-odd files in this repo — including internal/nscale's ResourceStatus
//     and every service model — are typed on common, and Go permits only one
//     version of a module per build. Adopting v0.2.0 for NKS therefore means
//     refactoring the type foundation under every existing resource in the same
//     change. That migration is worth doing, but on its own, behind the full
//     acceptance suite.
//
//  2. The SDK's vendored copy of the spec is already behind the canonical one
//     (it predates the platform-release organizationID filter and the required
//     usableOrganizationIds field), so pinning to it would ship a stale client.
//
// openapi.yaml here is a verbatim copy of the canonical spec at
// https://raw.githubusercontent.com/nscaledev/openapi/main/nks-core/main/openapi.yaml
// Refresh it and regenerate with `make nks-spec`.
//
// When the SDK-wide v0.2.0 migration lands, this package should be deleted and
// its import path swapped for nscale-sdk-go/kubernetes: the generated types are
// identical, because they come from the same spec through the same generator.

//go:generate go tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config config.yaml openapi.yaml
