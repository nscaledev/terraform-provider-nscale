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

// Type bridge between the legacy unikorn-cloud/core types this resource still
// consumes (via the deprecated unikorn-cloud/compute client) and the new
// nscale-sdk-go/common types that the rest of the provider — including all
// shared helpers in internal/nscale and the tftypes utilities — now uses.
//
// Once nscale_compute_cluster is removed (next breaking-change release), this
// file and the LegacyCompute client both go away.

package computecluster

import (
	common "github.com/nscaledev/nscale-sdk-go/common"
	legacycore "github.com/unikorn-cloud/core/pkg/openapi"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// readTagsToCommon converts a legacy tag list returned by the cluster API into
// the common-typed shape, filtering out internal operation tags via the shared
// helper.
func readTagsToCommon(in *legacycore.TagList) *common.TagList {
	return nscale.RemoveOperationTags(legacyTagsToCommon(in))
}

// writeTagsToLegacy filters operation tags from a common-typed list (typically
// produced by the tftypes helpers) and converts the result back to the legacy
// shape required by the cluster API request structs.
func writeTagsToLegacy(in *common.TagList) *legacycore.TagList {
	return commonTagsToLegacy(nscale.RemoveOperationTags(in))
}

// writeOperationTagLegacy mirrors nscale.WriteOperationTag for legacy metadata.
// WriteOperationTag mutates metadata.Tags, so we project the tags into a
// common shape, run the shared logic, then copy the result back so the caller
// sees the new tag on the original legacy metadata.
func writeOperationTagLegacy(metadata *legacycore.ResourceWriteMetadata) string {
	proxy := common.ResourceMetadata{
		Name:        metadata.Name,
		Description: metadata.Description,
		Tags:        legacyTagsToCommon(metadata.Tags),
	}
	key := nscale.WriteOperationTag(&proxy)
	metadata.Tags = commonTagsToLegacy(proxy.Tags)
	return key
}

// commonReadMetadataFromLegacy converts a legacy ProjectScopedResourceReadMetadata
// returned by the deprecated cluster API into the common-shape struct the
// shared state watchers in internal/nscale expect.
//
// Both types are openapi-generated and field-identical for the subset the
// state watcher and error-diagnostic helpers read (Id, Name, ProvisioningStatus,
// Tags). The remaining fields are copied for completeness.
func commonReadMetadataFromLegacy(
	in *legacycore.ProjectScopedResourceReadMetadata,
) *common.ProjectScopedResourceReadMetadata {
	if in == nil {
		return nil
	}
	return &common.ProjectScopedResourceReadMetadata{
		Id:                 in.Id,
		Name:               in.Name,
		Description:        in.Description,
		ProvisioningStatus: common.ResourceProvisioningStatus(in.ProvisioningStatus),
		HealthStatus:       common.ResourceHealthStatus(in.HealthStatus),
		CreatedBy:          in.CreatedBy,
		CreationTime:       in.CreationTime,
		DeletionTime:       in.DeletionTime,
		ModifiedBy:         in.ModifiedBy,
		ModifiedTime:       in.ModifiedTime,
		OrganizationId:     in.OrganizationId,
		ProjectId:          in.ProjectId,
		Tags:               legacyTagsToCommon(in.Tags),
	}
}

func legacyTagsToCommon(in *legacycore.TagList) *common.TagList {
	if in == nil {
		return nil
	}
	out := make(common.TagList, len(*in))
	for i, t := range *in {
		out[i] = common.Tag{Name: t.Name, Value: t.Value}
	}
	return &out
}

func commonTagsToLegacy(in *common.TagList) *legacycore.TagList {
	if in == nil {
		return nil
	}
	out := make(legacycore.TagList, len(*in))
	for i, t := range *in {
		out[i] = legacycore.Tag{Name: t.Name, Value: t.Value}
	}
	return &out
}
