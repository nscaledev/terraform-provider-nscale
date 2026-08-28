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

// ResourceStatus is the minimal view of read metadata the state watchers and
// reader depend on, decoupled from the SDK package that owns the resource.
type ResourceStatus struct {
	ID                 string
	Name               string
	ProvisioningStatus ProvisioningStatus
	// Tags is required by the update watcher, which polls until the operation
	// tag it wrote is observed on the resource.
	Tags *[]Tag
}

// ProvisioningStatus is the provider-neutral lifecycle state used by shared watchers.
type ProvisioningStatus string

// Tag is the provider-neutral tag shape used by shared state watchers.
type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// NewResourceStatus adapts service-local SDK metadata fields to ResourceStatus.
func NewResourceStatus[T tag, S ~string](id, name string, provisioningStatus S, tags *[]T) ResourceStatus {
	var statusTags *[]Tag
	if tags != nil {
		converted := make([]Tag, len(*tags))
		for i, tag := range *tags {
			converted[i] = Tag(tagValue(tag))
		}
		statusTags = &converted
	}

	return ResourceStatus{
		ID:                 id,
		Name:               name,
		ProvisioningStatus: ProvisioningStatus(provisioningStatus),
		Tags:               statusTags,
	}
}
