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

package filestorage

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

var (
	_ resource.ResourceWithConfigure   = &FileStorageResource{}
	_ resource.ResourceWithImportState = &FileStorageResource{}
)

type FileStorageResource struct {
	client *nscale.Client
}

func NewFileStorageResource() resource.Resource {
	return &FileStorageResource{}
}

func (r *FileStorageResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	client, ok := request.ProviderData.(*nscale.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configuration Type",
			fmt.Sprintf("Expected *nscale.Client, got: %T. Please contact the Nscale team for support.", request.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *FileStorageResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *FileStorageResource) Metadata(ctx context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_file_storage"
}

func (r *FileStorageResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale File Storage",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the file storage.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the file storage.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the file storage.",
				Optional:            true,
			},
			"storage_class_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the storage class used for the file storage.",
				Required:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "The amount of storage currently used, in gibibytes.",
				Computed:            true,
			},
			"capacity": schema.Int64Attribute{
				MarkdownDescription: "The total capacity requested for the file storage, in gibibytes.",
				Required:            true,
			},
			"root_squash": schema.BoolAttribute{
				MarkdownDescription: "Whether root squashing is applied to the file storage to restrict root access for clients.",
				Required:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the file storage is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the file storage was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"network": schema.ListNestedBlock{
				MarkdownDescription: "The network to which the file storage is attached.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The unique identifier of the network to attach the file storage to.",
							Required:            true,
						},
						"mount_source": schema.StringAttribute{
							MarkdownDescription: "The network path used to mount the file storage.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (r *FileStorageResource) setDefaultRegionID(data *FileStorageModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(r.client.RegionID)
	}
}

func (r *FileStorageResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleFileStorageCreateParams(r.client.OrganizationID, r.client.ProjectID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	fileStorageCreateResponse, err := r.client.Region.PostApiV2Filestorage(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create File Storage",
			fmt.Sprintf("An error occurred while creating the file storage: %s", err),
		)
		return
	}

	fileStorage, err := nscale.ReadJSONResponsePointer[regionapi.StorageV2Read](fileStorageCreateResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create File Storage",
			fmt.Sprintf("An error occurred while creating the file storage: %s", err),
		)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[regionapi.StorageV2Read]{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context) (*regionapi.StorageV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			targetID := fileStorage.Metadata.Id
			return getFileStorage(ctx, targetID, r.client)
		},
	}

	fileStorage, ok := stateWatcher.Wait(ctx, response)
	if !ok {
		return
	}

	data = NewFileStorageModel(fileStorage)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *FileStorageResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[regionapi.StorageV2Read]{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context, id string) (*regionapi.StorageV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getFileStorage(ctx, id, r.client)
		},
	}

	fileStorage, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data = NewFileStorageModel(fileStorage)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *FileStorageResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleFileStorageUpdateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	fileStorageUpdateResponse, err := r.client.Region.PutApiV2FilestorageFilestorageID(ctx, id, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update File Storage",
			fmt.Sprintf("An error occurred while updating the file storage: %s", err),
		)
		return
	}

	fileStorage, err := nscale.ReadJSONResponsePointer[regionapi.StorageV2Read](fileStorageUpdateResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update File Storage",
			fmt.Sprintf("An error occurred while updating the file storage: %s", err),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[regionapi.StorageV2Read]{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context) (*regionapi.StorageV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getFileStorage(ctx, id, r.client)
		},
	}

	fileStorage, ok := stateWatcher.Wait(ctx, operationTagKey, response)
	if !ok {
		return
	}

	data = NewFileStorageModel(fileStorage)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *FileStorageResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	fileStorageDeleteResponse, err := r.client.Region.DeleteApiV2FilestorageFilestorageID(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete File Storage",
			fmt.Sprintf("An error occurred while deleting the file storage: %s", err),
		)
		return
	}

	if err = nscale.ReadErrorResponse(fileStorageDeleteResponse); err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete File Storage",
			fmt.Sprintf("An error occurred while deleting the file storage: %s", err),
		)
		return
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getFileStorage(ctx, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, response)
}
