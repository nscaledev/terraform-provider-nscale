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
	"context"
	"fmt"
	"net/http"
	"strings"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.ResourceWithConfigure   = &ObjectStorageAccessKeyResource{}
	_ resource.ResourceWithImportState = &ObjectStorageAccessKeyResource{}
)

type ObjectStorageAccessKeyResourceModel struct {
	ObjectStorageAccessKeyModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

type ObjectStorageAccessKeyResource struct {
	client *nscale.Client
}

func NewObjectStorageAccessKeyResource() resource.Resource {
	return &ObjectStorageAccessKeyResource{}
}

func (r *ObjectStorageAccessKeyResource) Configure(
	ctx context.Context,
	request resource.ConfigureRequest,
	response *resource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	client, ok := request.ProviderData.(*nscale.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configuration Type",
			fmt.Sprintf(
				"Expected *nscale.Client, got: %T. Please contact the Nscale team for support.",
				request.ProviderData,
			),
		)
		return
	}

	r.client = client
}

func (r *ObjectStorageAccessKeyResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_object_storage_access_key"
}

func (r *ObjectStorageAccessKeyResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "An S3-compatible access key bound to an object storage endpoint. The access key's secret is returned only at creation; protect Terraform state accordingly. The resource is immutable — any change forces replacement, which generates a fresh secret.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the access key.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the object storage endpoint this access key belongs to. The endpoint is immutable for an access key — changing it forces replacement.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the access key.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the access key.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"identity_policy": schema.StringAttribute{
				MarkdownDescription: "The name of an identity policy configured on the parent endpoint that this key is bound to.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access_key_id": schema.StringAttribute{
				MarkdownDescription: "The S3 access key identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"secret": schema.StringAttribute{
				MarkdownDescription: "The S3 secret access key. Returned only when the access key is created and never re-read from the API. Treat as write-once: store it in a secret manager, or replace the resource (`terraform apply -replace=...`) to obtain a new value.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the access key is provisioned. If not specified, this defaults to the project ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the access key was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *ObjectStorageAccessKeyResource) setDefaultIDs(data *ObjectStorageAccessKeyResourceModel) {
	if data.ProjectID.ValueString() == "" {
		data.ProjectID = types.StringValue(r.client.ProjectID)
	}
}

func (r *ObjectStorageAccessKeyResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageAccessKeyResourceModel](
		ctx,
		request.Plan.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	endpointID := data.EndpointID.ValueString()
	params := data.NscaleObjectStorageAccessKeyCreateParams()

	createResponse, err := r.client.Storage.PostApiV1ObjectstorageendpointsObjectStorageEndpointIDAccesskeys(
		ctx,
		endpointID,
		params,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Object Storage Access Key",
			fmt.Sprintf("An error occurred while creating the access key: %s", err),
		)
		return
	}
	defer createResponse.Body.Close()

	created, err := nscale.ReadJSONResponsePointer[storageapi.ObjectStorageAccessKeyCreateResponseBody](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Object Storage Access Key",
			fmt.Sprintf("An error occurred while creating the access key: %s", err),
		)
		return
	}

	if created.Spec.Secret == "" {
		response.Diagnostics.AddError(
			"Missing Secret in Create Response",
			"The Nscale API did not return a secret in the create response. This is unrecoverable: "+
				"the access key has been created but the secret cannot be retrieved later. Delete the "+
				"access key (via `terraform destroy` or the dashboard) and contact Nscale support.",
		)
		return
	}

	// Persist what we have to state immediately, so a watcher failure later
	// doesn't strand the secret.
	data.ObjectStorageAccessKeyModel = NewObjectStorageAccessKeyModelFromCreate(created)
	data.EndpointID = types.StringValue(endpointID)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[storageapi.ObjectStorageAccessKeyRead]{
		ResourceTitle: "Object Storage Access Key",
		ResourceName:  "object_storage_access_key",
		GetFunc: func(ctx context.Context) (*storageapi.ObjectStorageAccessKeyRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getObjectStorageAccessKey(ctx, endpointID, created.Metadata.Id, r.client)
		},
	}

	settled, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
		return
	}

	// Capture the secret from the create response — the settled Read does
	// not include it. EndpointID is also a Terraform-only attribute and not
	// in the Read response, so we re-attach both.
	preservedSecret := data.Secret
	data.ObjectStorageAccessKeyModel = NewObjectStorageAccessKeyModel(settled)
	data.Secret = preservedSecret
	data.EndpointID = types.StringValue(endpointID)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ObjectStorageAccessKeyResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageAccessKeyResourceModel](
		ctx,
		request.State.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	preservedSecret := data.Secret
	preservedEndpointID := data.EndpointID

	resourceReader := nscale.ResourceReader[storageapi.ObjectStorageAccessKeyRead]{
		ResourceTitle: "Object Storage Access Key",
		ResourceName:  "object_storage_access_key",
		GetFunc: func(ctx context.Context, id string) (*storageapi.ObjectStorageAccessKeyRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getObjectStorageAccessKey(ctx, preservedEndpointID.ValueString(), id, r.client)
		},
	}

	accessKey, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data.ObjectStorageAccessKeyModel = NewObjectStorageAccessKeyModel(accessKey)
	data.Secret = preservedSecret
	data.EndpointID = preservedEndpointID
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ObjectStorageAccessKeyResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	// Defence in depth: every configurable attribute carries RequiresReplace,
	// so the framework should never call Update. Reject explicitly so any
	// future attribute that drops RequiresReplace by mistake fails loudly
	// instead of silently no-op'ing.
	response.Diagnostics.AddError(
		"Update Not Supported",
		"Object storage access keys are immutable and cannot be updated in-place. All changes require resource replacement.",
	)
}

func (r *ObjectStorageAccessKeyResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageAccessKeyResourceModel](
		ctx,
		request.State.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	endpointID := data.EndpointID.ValueString()
	id := data.ID.ValueString()

	deleteResponse, err := r.client.Storage.DeleteApiV1ObjectstorageendpointsObjectStorageEndpointIDAccesskeysObjectStorageAccessKeyID(
		ctx,
		endpointID,
		id,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Object Storage Access Key",
			fmt.Sprintf("An error occurred while deleting the access key: %s", err),
		)
		return
	}
	defer deleteResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(deleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			nscale.TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				"Failed to Delete Object Storage Access Key",
				fmt.Sprintf("An error occurred while deleting the access key: %s", err),
			)
			return
		}
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Object Storage Access Key",
		ResourceName:  "object_storage_access_key",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getObjectStorageAccessKey(ctx, endpointID, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}

// ImportState parses a composite ID of the form "<endpoint_id>/<access_key_id>"
// since access keys are scoped to an endpoint and the API never returns the
// endpoint id on the access key resource itself. Emits a warning that the
// secret cannot be recovered through import.
func (r *ObjectStorageAccessKeyResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	const importIDParts = 2
	parts := strings.SplitN(request.ID, "/", importIDParts)
	if len(parts) != importIDParts || parts[0] == "" || parts[1] == "" {
		response.Diagnostics.AddError(
			"Invalid Import ID",
			"Import ID must be of the form '<endpoint_id>/<access_key_id>'.",
		)
		return
	}

	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("endpoint_id"), parts[0])...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
	if response.Diagnostics.HasError() {
		return
	}

	response.Diagnostics.AddWarning(
		"Secret Cannot Be Imported",
		"The Nscale API only returns the access key secret at creation time. After import, "+
			"`secret` will be null in state. To recover the secret, replace the resource "+
			"with `terraform apply -replace=...`.",
	)
}
