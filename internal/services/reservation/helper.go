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

package reservation

import (
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// statusOf projects a read's metadata onto the ResourceStatus the shared
// watchers consume.
//
// Reservations and placements are both project-scoped and carry the same
// metadata type, so the field mapping is written once here rather than repeated
// in each resource's get helper.
func statusOf(metadata *reservationapi.ProjectScopedResourceReadMetadata) nscale.ResourceStatus {
	return nscale.NewResourceStatus(
		metadata.Id,
		metadata.Name,
		metadata.ProvisioningStatus,
		metadata.Tags,
	)
}
