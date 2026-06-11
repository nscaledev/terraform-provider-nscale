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
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.ResourceWithConfigure   = &ObjectStorageEndpointResource{}
	_ resource.ResourceWithImportState = &ObjectStorageEndpointResource{}
)

type ObjectStorageEndpointResourceModel struct {
	ObjectStorageEndpointModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

type ObjectStorageEndpointResource struct {
	client *nscale.Client
}

func NewObjectStorageEndpointResource() resource.Resource {
	return &ObjectStorageEndpointResource{}
}

func (r *ObjectStorageEndpointResource) Configure(
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

func (r *ObjectStorageEndpointResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *ObjectStorageEndpointResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_object_storage_endpoint"
}

func (r *ObjectStorageEndpointResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "An S3-compatible object storage endpoint provisioned in an Nscale project. The endpoint exposes a chosen endpoint class's connectivity (public DNS today; private may be added in future) and carries one or more identity policies that govern what its access keys are allowed to do.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the object storage endpoint.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the object storage endpoint.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the object storage endpoint.",
				Optional:            true,
			},
			"endpoint_class_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the endpoint class that determines the endpoint's exposure type and regional availability. The endpoint class is immutable after creation.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the object storage endpoint.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"identity_policies": schema.ListNestedAttribute{
				MarkdownDescription: "Identity policies configured on the endpoint. Policy names must be unique within the endpoint. Updating this attribute replaces the full set of configured policies.",
				Optional:            true,
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The identity policy name. Must be unique within this endpoint.",
							Required:            true,
						},
						"document": schema.StringAttribute{
							MarkdownDescription: "A provider-specific identity policy document encoded as a JSON object string. Use Terraform's `jsonencode()` to construct it. Whitespace and key ordering are normalised so equivalent documents do not produce diffs.",
							Required:            true,
							PlanModifiers: []planmodifier.String{
								normalizeJSONPlanModifier{},
							},
						},
					},
				},
			},
			"exposure": schema.SingleNestedAttribute{
				MarkdownDescription: "The externally reachable endpoints for the object storage endpoint, populated once provisioning completes.",
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"public": schema.SingleNestedAttribute{
						MarkdownDescription: "Connection details for the public exposure mode, when the endpoint class supports it.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"dns_name": schema.StringAttribute{
								MarkdownDescription: "The DNS hostname clients use to reach the endpoint over the public network.",
								Computed:            true,
							},
						},
					},
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the object storage endpoint is provisioned. If not specified, this defaults to the project ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the object storage endpoint is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the object storage endpoint was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// setDefaultIDs fills the region ID from the provider configuration when the plan
// leaves it empty. The project ID is resolved separately at create (see Create)
// because an unresolved project ID must raise an error rather than silently default.
func (r *ObjectStorageEndpointResource) setDefaultIDs(data *ObjectStorageEndpointResourceModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(r.client.RegionID)
	}
}

func (r *ObjectStorageEndpointResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageEndpointResourceModel](
		ctx,
		request.Plan.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	projectID, diagnostics := r.client.ResolveProjectID(data.ProjectID.ValueString())
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}
	data.ProjectID = types.StringValue(projectID)

	params, diagnostics := data.NscaleObjectStorageEndpointCreateParams(ctx, r.client.OrganizationID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	createResponse, err := r.client.Storage.PostApiV1Objectstorageendpoints(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Object Storage Endpoint",
			fmt.Sprintf("An error occurred while creating the object storage endpoint: %s", err),
		)
		return
	}
	defer createResponse.Body.Close()

	endpoint, err := nscale.ReadJSONResponsePointer[storageapi.ObjectStorageEndpointRead](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create Object Storage Endpoint",
			fmt.Sprintf("An error occurred while creating the object storage endpoint: %s", err),
		)
		return
	}

	endpointModel, modelDiags := NewObjectStorageEndpointModel(endpoint)
	if modelDiags.HasError() {
		response.Diagnostics.Append(modelDiags...)
		return
	}
	data.ObjectStorageEndpointModel = endpointModel
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[storageapi.ObjectStorageEndpointRead]{
		ResourceTitle: "Object Storage Endpoint",
		ResourceName:  "object_storage_endpoint",
		GetFunc: func(ctx context.Context) (*storageapi.ObjectStorageEndpointRead, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getObjectStorageEndpoint(ctx, endpoint.Metadata.Id, r.client))
		},
	}

	settled, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
		return
	}

	settledModel, modelDiags := NewObjectStorageEndpointModel(settled)
	if modelDiags.HasError() {
		response.Diagnostics.Append(modelDiags...)
		return
	}
	data.ObjectStorageEndpointModel = settledModel
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ObjectStorageEndpointResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageEndpointResourceModel](
		ctx,
		request.State.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[storageapi.ObjectStorageEndpointRead]{
		ResourceTitle: "Object Storage Endpoint",
		ResourceName:  "object_storage_endpoint",
		GetFunc: func(ctx context.Context, id string) (*storageapi.ObjectStorageEndpointRead, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getObjectStorageEndpoint(ctx, id, r.client))
		},
	}

	endpoint, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	endpointModel, modelDiags := NewObjectStorageEndpointModel(endpoint)
	if modelDiags.HasError() {
		response.Diagnostics.Append(modelDiags...)
		return
	}
	data.ObjectStorageEndpointModel = endpointModel
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ObjectStorageEndpointResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageEndpointResourceModel](
		ctx,
		request.Plan.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleObjectStorageEndpointUpdateParams(ctx)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	updateResponse, err := r.client.Storage.PutApiV1ObjectstorageendpointsObjectStorageEndpointID(ctx, id, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Object Storage Endpoint",
			fmt.Sprintf("An error occurred while updating the object storage endpoint: %s", err),
		)
		return
	}
	defer updateResponse.Body.Close()

	if _, readErr := nscale.ReadJSONResponsePointer[storageapi.ObjectStorageEndpointRead](
		updateResponse,
	); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Update Object Storage Endpoint",
			fmt.Sprintf("An error occurred while updating the object storage endpoint: %s", readErr),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[storageapi.ObjectStorageEndpointRead]{
		ResourceTitle: "Object Storage Endpoint",
		ResourceName:  "object_storage_endpoint",
		GetFunc: func(ctx context.Context) (*storageapi.ObjectStorageEndpointRead, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getObjectStorageEndpoint(ctx, id, r.client))
		},
	}

	settled, ok := stateWatcher.Wait(ctx, operationTagKey, data.Timeouts, response)
	if !ok {
		return
	}

	settledModel, modelDiags := NewObjectStorageEndpointModel(settled)
	if modelDiags.HasError() {
		response.Diagnostics.Append(modelDiags...)
		return
	}
	data.ObjectStorageEndpointModel = settledModel
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *ObjectStorageEndpointResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[ObjectStorageEndpointResourceModel](
		ctx,
		request.State.Get,
		r.setDefaultIDs,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	deleteResponse, err := r.client.Storage.DeleteApiV1ObjectstorageendpointsObjectStorageEndpointID(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Object Storage Endpoint",
			fmt.Sprintf("An error occurred while deleting the object storage endpoint: %s", err),
		)
		return
	}
	defer deleteResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(deleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			nscale.TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				"Failed to Delete Object Storage Endpoint",
				fmt.Sprintf("An error occurred while deleting the object storage endpoint: %s", err),
			)
			return
		}
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Object Storage Endpoint",
		ResourceName:  "object_storage_endpoint",
		GetFunc: func(ctx context.Context) (any, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getObjectStorageEndpoint(ctx, id, r.client))
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}

// normalizeJSONPlanModifier suppresses diffs between plan and state when the
// only difference is whitespace or key ordering inside an identity policy
// document. Without this, Terraform's `jsonencode()` output (which has a
// stable but verbose shape) would never match the API's compacted form,
// producing perpetual diffs.
type normalizeJSONPlanModifier struct{}

func (m normalizeJSONPlanModifier) Description(_ context.Context) string {
	return "Suppresses diffs between semantically equivalent JSON documents."
}

func (m normalizeJSONPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m normalizeJSONPlanModifier) PlanModifyString(
	_ context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	var planDoc, stateDoc any
	if err := json.Unmarshal([]byte(req.PlanValue.ValueString()), &planDoc); err != nil {
		return
	}
	if err := json.Unmarshal([]byte(req.StateValue.ValueString()), &stateDoc); err != nil {
		return
	}
	if reflect.DeepEqual(planDoc, stateDoc) {
		resp.PlanValue = req.StateValue
	}
}
