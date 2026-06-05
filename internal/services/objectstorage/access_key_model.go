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

package objectstorage

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"
)

// ObjectStorageAccessKeyModel is the Terraform-side model. EndpointID is a
// Terraform-only attribute (the API never returns it on the access key
// resource itself; it's encoded in the path) so callers must preserve it
// across model rebuilds.
//
// Secret is a create-only attribute — the API only returns it in the create
// response and never on subsequent reads. The resource captures it from the
// create response and re-attaches it to state on every Read; the model
// converter intentionally leaves it as the zero value.
type ObjectStorageAccessKeyModel struct {
	ID             types.String `tfsdk:"id"`
	EndpointID     types.String `tfsdk:"endpoint_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	IdentityPolicy types.String `tfsdk:"identity_policy"`
	AccessKeyID    types.String `tfsdk:"access_key_id"`
	Secret         types.String `tfsdk:"secret"`
	ProjectID      types.String `tfsdk:"project_id"`
	CreationTime   types.String `tfsdk:"creation_time"`
}

// NewObjectStorageAccessKeyModel maps a read-shape API response into the
// Terraform model. It deliberately leaves `Secret` and `EndpointID` as the
// zero value: those are caller-managed (see the resource's Create/Read).
func NewObjectStorageAccessKeyModel(source *storageapi.ObjectStorageAccessKeyRead) ObjectStorageAccessKeyModel {
	return ObjectStorageAccessKeyModel{
		ID:             types.StringValue(source.Metadata.Id),
		Name:           types.StringValue(source.Metadata.Name),
		Description:    types.StringPointerValue(source.Metadata.Description),
		IdentityPolicy: types.StringValue(source.Spec.IdentityPolicy),
		AccessKeyID:    types.StringPointerValue(source.Spec.AccessKeyId),
		ProjectID:      types.StringValue(source.Metadata.ProjectId),
		CreationTime:   types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

// NewObjectStorageAccessKeyModelFromCreate maps the create-shape API response
// (which carries the secret) into the Terraform model. The caller is
// responsible for setting EndpointID afterwards — it is not in the response.
func NewObjectStorageAccessKeyModelFromCreate(
	source *storageapi.ObjectStorageAccessKeyCreateResponseBody,
) ObjectStorageAccessKeyModel {
	return ObjectStorageAccessKeyModel{
		ID:             types.StringValue(source.Metadata.Id),
		Name:           types.StringValue(source.Metadata.Name),
		Description:    types.StringPointerValue(source.Metadata.Description),
		IdentityPolicy: types.StringValue(source.Spec.IdentityPolicy),
		AccessKeyID:    types.StringValue(source.Spec.AccessKeyId),
		Secret:         types.StringValue(source.Spec.Secret),
		ProjectID:      types.StringValue(source.Metadata.ProjectId),
		CreationTime:   types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

// NscaleObjectStorageAccessKeyCreateParams converts a planned model into the
// API create request body. The Secret is generated server-side, so it has no
// place in the request.
func (m *ObjectStorageAccessKeyModel) NscaleObjectStorageAccessKeyCreateParams() storageapi.ObjectStorageAccessKeyCreate {
	return storageapi.ObjectStorageAccessKeyCreate{
		Metadata: coreapi.ResourceWriteMetadata{
			Name:        m.Name.ValueString(),
			Description: m.Description.ValueStringPointer(),
			Tags:        nil,
		},
		Spec: storageapi.ObjectStorageAccessKeyCreateSpec{
			IdentityPolicy: m.IdentityPolicy.ValueString(),
		},
	}
}
