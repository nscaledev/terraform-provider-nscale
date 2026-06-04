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

package nscale

import coreapi "github.com/nscaledev/nscale-sdk-go/common"

// ResourceStatus is the minimal view of read metadata the state watchers and
// reader depend on, decoupled from whether a resource is project- or
// organization-scoped. The two concrete SDK metadata shapes
// (ProjectScopedResourceReadMetadata / OrganizationScopedResourceReadMetadata)
// are field-identical except for ProjectId, so both project- and org-scoped
// resources can share a single watcher implementation via the adapters below.
type ResourceStatus struct {
	ID                 string
	Name               string
	ProvisioningStatus coreapi.ResourceProvisioningStatus
	// Tags is required by the update watcher, which polls until the operation
	// tag it wrote is observed on the resource.
	Tags *coreapi.TagList
}

// StatusFromProjectScoped adapts project-scoped read metadata to ResourceStatus.
// It is nil-safe: get helpers return nil metadata on error, and the watchers
// only read the status on a successful refresh.
func StatusFromProjectScoped(m *coreapi.ProjectScopedResourceReadMetadata) ResourceStatus {
	if m == nil {
		return ResourceStatus{}
	}

	return ResourceStatus{
		ID:                 m.Id,
		Name:               m.Name,
		ProvisioningStatus: m.ProvisioningStatus,
		Tags:               m.Tags,
	}
}

// StatusFromOrgScoped adapts organization-scoped read metadata to ResourceStatus.
// It is nil-safe for the same reason as StatusFromProjectScoped.
func StatusFromOrgScoped(m *coreapi.OrganizationScopedResourceReadMetadata) ResourceStatus {
	if m == nil {
		return ResourceStatus{}
	}

	return ResourceStatus{
		ID:                 m.Id,
		Name:               m.Name,
		ProvisioningStatus: m.ProvisioningStatus,
		Tags:               m.Tags,
	}
}

// AdaptProjectScoped wires a project-scoped get helper's (resource, metadata, error)
// return triple onto the (resource, ResourceStatus, error) shape the shared watchers
// and reader expect. Passing the get call straight through keeps each watcher closure
// a single expression with no intermediate error variable to shadow.
func AdaptProjectScoped[T any](
	result *T,
	metadata *coreapi.ProjectScopedResourceReadMetadata,
	err error,
) (*T, ResourceStatus, error) {
	return result, StatusFromProjectScoped(metadata), err
}

// AdaptOrgScoped is the organization-scoped counterpart to AdaptProjectScoped.
func AdaptOrgScoped[T any](
	result *T,
	metadata *coreapi.OrganizationScopedResourceReadMetadata,
	err error,
) (*T, ResourceStatus, error) {
	return result, StatusFromOrgScoped(metadata), err
}
