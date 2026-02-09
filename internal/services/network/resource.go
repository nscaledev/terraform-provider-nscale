package network

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
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
	_ resource.ResourceWithConfigure   = &NetworkResource{}
	_ resource.ResourceWithImportState = &NetworkResource{}
)

type NetworkResource struct {
	client *nscale.Client
}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{}
}

func (r *NetworkResource) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
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

func (r *NetworkResource) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *NetworkResource) Metadata(ctx context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_network"
}

func (r *NetworkResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Network",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the network.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the network.",
				Required:            true,
				Validators: []validator.String{
					validators.NameValidator(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the network.",
				Optional:            true,
			},
			"dns_nameservers": schema.ListAttribute{
				MarkdownDescription: "A list of DNS nameservers to configure for the network.",
				ElementType:         types.StringType,
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(validators.IPAddressValidator{}),
				},
			},
			"routes": schema.ListNestedAttribute{
				MarkdownDescription: "A list of routes for the network.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"destination": schema.StringAttribute{
							MarkdownDescription: "The destination CIDR block for the route.",
							Required:            true,
							Validators: []validator.String{
								validators.CIDRValidator{},
							},
						},
						"nexthop": schema.StringAttribute{
							MarkdownDescription: "The next-hop address for the route.",
							Required:            true,
							Validators: []validator.String{
								validators.IPAddressValidator{},
							},
						},
					},
				},
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
			},
			"cidr_block": schema.StringAttribute{
				MarkdownDescription: "The CIDR block assigned to the network.",
				Required:            true,
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the network.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the network is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the network was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *NetworkResource) setDefaultRegionID(data *NetworkModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(r.client.RegionID)
	}
}

func (r *NetworkResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	data, diagnostics := nscale.ReadTerraformState[NetworkModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleNetworkCreateParams(r.client.OrganizationID, r.client.ProjectID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	networkCreateResponse, err := r.client.Region.PostApiV2Networks(ctx, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Network",
			fmt.Sprintf("An error occurred while creating the network: %s", err),
		)
		return
	}

	network, err := nscale.ReadJSONResponsePointerWithContext[regionapi.NetworkV2Read](ctx, networkCreateResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Create Network",
			fmt.Sprintf("An error occurred while creating the network: %s", err),
		)
		return
	}

	data = NewNetworkModel(network)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[regionapi.NetworkV2Read]{
		ResourceTitle: "Network",
		ResourceName:  "network",
		GetFunc: func(ctx context.Context) (*regionapi.NetworkV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			targetID := network.Metadata.Id
			return getNetwork(ctx, targetID, r.client)
		},
	}

	network, ok := stateWatcher.Wait(ctx, response)
	if !ok {
		return
	}

	data = NewNetworkModel(network)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *NetworkResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[NetworkModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	resourceReader := nscale.ResourceReader[regionapi.NetworkV2Read]{
		ResourceTitle: "Network",
		ResourceName:  "network",
		GetFunc: func(ctx context.Context, id string) (*regionapi.NetworkV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getNetwork(ctx, id, r.client)
		},
	}

	network, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data = NewNetworkModel(network)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *NetworkResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	data, diagnostics := nscale.ReadTerraformState[NetworkModel](ctx, request.Plan.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	params, diagnostics := data.NscaleNetworkUpdateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	networkUpdateResponse, err := r.client.Region.PutApiV2NetworksNetworkID(ctx, id, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Network",
			fmt.Sprintf("An error occurred while updating the network: %s", err),
		)
		return
	}

	network, err := nscale.ReadJSONResponsePointerWithContext[regionapi.NetworkV2Read](ctx, networkUpdateResponse)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update Network",
			fmt.Sprintf("An error occurred while updating the network: %s", err),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[regionapi.NetworkV2Read]{
		ResourceTitle: "Network",
		ResourceName:  "network",
		GetFunc: func(ctx context.Context) (*regionapi.NetworkV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getNetwork(ctx, id, r.client)
		},
	}

	network, ok := stateWatcher.Wait(ctx, operationTagKey, response)
	if !ok {
		return
	}

	data = NewNetworkModel(network)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *NetworkResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	data, diagnostics := nscale.ReadTerraformState[NetworkModel](ctx, request.State.Get, r.setDefaultRegionID)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	networkDeleteResponse, err := r.client.Region.DeleteApiV2NetworksNetworkID(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete Network",
			fmt.Sprintf("An error occurred while deleting the network: %s", err),
		)
		return
	}

	if err = nscale.ReadEmptyResponseWithContext(ctx, networkDeleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			response.Diagnostics.AddError(
				"Failed to Delete Network",
				fmt.Sprintf("An error occurred while deleting the network: %s", err),
			)
			return
		}
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "Network",
		ResourceName:  "network",
		GetFunc: func(ctx context.Context) (any, *coreapi.ProjectScopedResourceReadMetadata, error) {
			return getNetwork(ctx, id, r.client)
		},
	}

	stateWatcher.Wait(ctx, response)
}
