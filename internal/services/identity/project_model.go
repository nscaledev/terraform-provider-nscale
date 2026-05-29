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

package identity

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
)

type ProjectModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Tags               types.Map    `tfsdk:"tags"`
	GroupIDs           types.Set    `tfsdk:"group_ids"`
	CreationTime       types.String `tfsdk:"creation_time"`
	ProvisioningStatus types.String `tfsdk:"provisioning_status"`
}

func NewProjectModel(source *identityapi.ProjectRead) ProjectModel {
	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	groupIDs := make([]attr.Value, 0, len(source.Spec.GroupIDs))
	for _, groupID := range source.Spec.GroupIDs {
		groupIDs = append(groupIDs, types.StringValue(groupID))
	}

	return ProjectModel{
		ID:          types.StringValue(source.Metadata.Id),
		Name:        types.StringValue(source.Metadata.Name),
		Description: types.StringPointerValue(source.Metadata.Description),
		Tags:        tftypes.TagMapValueMust(tags),
		// Faithful: the API returns groupIDs as `[]` (never null), so an empty
		// configured set must round-trip as an empty set, not null.
		GroupIDs:           types.SetValueMust(types.StringType, groupIDs),
		CreationTime:       types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
		ProvisioningStatus: types.StringValue(string(source.Metadata.ProvisioningStatus)),
	}
}

func (m *ProjectModel) groupIDs() ([]string, diag.Diagnostics) {
	groupIDs := []string{}
	if diagnostics := m.GroupIDs.ElementsAs(context.TODO(), &groupIDs, false); diagnostics.HasError() {
		return nil, diagnostics
	}
	if groupIDs == nil {
		groupIDs = []string{}
	}
	return groupIDs, nil
}

func (m *ProjectModel) NscaleProjectCreateParams() (identityapi.ProjectWrite, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return identityapi.ProjectWrite{}, diagnostics
	}
	tags = nscale.RemoveOperationTags(tags)

	groupIDs, diagnostics := m.groupIDs()
	if diagnostics.HasError() {
		return identityapi.ProjectWrite{}, diagnostics
	}

	return identityapi.ProjectWrite{
		Metadata: coreapi.ResourceWriteMetadata{
			Name:        m.Name.ValueString(),
			Description: m.Description.ValueStringPointer(),
			Tags:        tags,
		},
		Spec: identityapi.ProjectSpec{
			GroupIDs: groupIDs,
		},
	}, nil
}

// NscaleProjectUpdateParams produces the PUT body. The identity project API
// uses the same ProjectWrite shape for create and update, so this mirrors the
// create path; it exists as its own method so the update flow can attach an
// operation tag without disturbing create.
func (m *ProjectModel) NscaleProjectUpdateParams() (identityapi.ProjectWrite, diag.Diagnostics) {
	return m.NscaleProjectCreateParams()
}
