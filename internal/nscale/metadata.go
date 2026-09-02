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

import "github.com/nscaledev/terraform-provider-nscale/internal/utils/tags"

// Tag re-exports the provider's own tag type so a service package that already
// imports nscale needs no second import to build a request body. It is an alias
// rather than a new type, so values cross between the two names freely.
type Tag = tags.Tag

// TagList re-exports the provider's tag slice, on the same terms as Tag.
type TagList = tags.List

// TagsFromAPI and TagsToAPI convert between a service package's generated tag
// type and the provider's own. Re-exported alongside the aliases above so a
// model file that already imports nscale needs no second import, and so a local
// variable named tags cannot shadow the package.
func TagsFromAPI[T tags.SDKTag](in *[]T) *TagList { return tags.FromAPI(in) }

func TagsToAPI[T tags.SDKTag](in *TagList) *[]T { return tags.ToAPI[T](in) }

// ProvisioningStatus is the provider's copy of the API's resource provisioning
// status.
//
// Owned here rather than taken from the SDK because each service package
// declares its own: the published specs are bundled with every $ref
// dereferenced, so no type is shared between them. The shared watchers below
// compare statuses across every service, which a per-service type cannot
// express.
type ProvisioningStatus string

const (
	ProvisioningStatusDeprovisioning ProvisioningStatus = "deprovisioning"
	ProvisioningStatusError          ProvisioningStatus = "error"
	ProvisioningStatusPending        ProvisioningStatus = "pending"
	ProvisioningStatusProvisioned    ProvisioningStatus = "provisioned"
	ProvisioningStatusProvisioning   ProvisioningStatus = "provisioning"
	ProvisioningStatusUnknown        ProvisioningStatus = "unknown"
)

// ResourceStatus is the minimal view of read metadata the state watchers and
// reader depend on, decoupled from both the owning service package and whether a
// resource is project- or organization-scoped.
//
// Each service's get helper builds this from its own metadata, at the boundary
// where the concrete SDK type is still in scope. That keeps the SDK's
// per-service types out of the shared layer entirely — the alternative, an
// adapter per (service, scope) pair, multiplies with no benefit, and the metadata
// shapes are not even uniform: reservation carries provisioning and health
// status detail the other services do not.
type ResourceStatus struct {
	ID                 string
	Name               string
	ProvisioningStatus ProvisioningStatus
	// Tags is required by the update watcher, which polls until the operation
	// tag it wrote is observed on the resource.
	Tags *TagList
}

// NewResourceStatus projects a service's read metadata onto ResourceStatus.
//
// Both type parameters are inferred from the metadata's own fields, so a caller
// reads as a single line regardless of which service package — or which scope's
// metadata — it holds:
//
//	return network, nscale.NewResourceStatus(m.Id, m.Name, m.ProvisioningStatus, m.Tags), nil
//
// S is constrained to ~string rather than a concrete type because each service
// declares its own defined provisioning-status type over string.
func NewResourceStatus[S ~string, T tags.SDKTag](
	id, name string,
	provisioningStatus S,
	tagList *[]T,
) ResourceStatus {
	return ResourceStatus{
		ID:                 id,
		Name:               name,
		ProvisioningStatus: ProvisioningStatus(provisioningStatus),
		Tags:               TagsFromAPI(tagList),
	}
}
