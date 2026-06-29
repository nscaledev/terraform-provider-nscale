package network

import (
	"context"
	"fmt"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	regionids "github.com/unikorn-cloud/region/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.Resource                = &NetworkResource{}
	_ resource.ResourceWithConfigure   = &NetworkResource{}
	_ resource.ResourceWithImportState = &NetworkResource{}
)

type NetworkResourceModel struct {
	NetworkModel

	Timeouts tftimeouts.Value `tfsdk:"timeouts"`
}

// NetworkResource embeds the generic CRUD base; only Schema and the adapter
// wiring below are network-specific.
type NetworkResource struct {
	*nscale.GenericResource[NetworkResourceModel, regionapi.NetworkV2Read]
}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{
		GenericResource: nscale.NewGenericResource(networkAdapter()),
	}
}

// networkAdapter wires the network-specific SDK calls and model mapping into the
// generic resource skeleton.
func networkAdapter() nscale.ResourceAdapter[NetworkResourceModel, regionapi.NetworkV2Read] {
	return nscale.ResourceAdapter[NetworkResourceModel, regionapi.NetworkV2Read]{
		TypeNameSuffix: "_network",
		Title:          "Network",
		Name:           "network",
		Create:         networkCreate,
		Update:         networkUpdate,
		Delete:         networkDelete,
		Get: func(
			ctx context.Context,
			client *nscale.Client,
			id string,
		) (*regionapi.NetworkV2Read, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getNetwork(ctx, id, client))
		},
		ToModel: func(api *regionapi.NetworkV2Read, dst *NetworkResourceModel) {
			dst.NetworkModel = NewNetworkModel(api)
		},
		IDFromModel:       func(m NetworkResourceModel) string { return m.ID.ValueString() },
		TimeoutsFromModel: func(m NetworkResourceModel) tftimeouts.Value { return m.Timeouts },
	}
}

func (r *NetworkResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Network",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the network.",
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
				Validators: []validator.String{
					validators.CIDRValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the network is provisioned. If not specified, this defaults to the project ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the network is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the network was created.",
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

// setDefaultIDs resolves the project and region IDs for a create. The project ID
// falls back to the provider default and errors when neither it nor a resource
// value is set; the region ID falls back silently. Only meaningful at create time.
func setDefaultIDs(client *nscale.Client, data *NetworkResourceModel) diag.Diagnostics {
	projectID, diagnostics := client.ResolveProjectID(data.ProjectID.ValueString())
	if diagnostics.HasError() {
		return diagnostics
	}
	data.ProjectID = types.StringValue(projectID)

	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(client.RegionID)
	}

	return diagnostics
}

func networkCreate(
	ctx context.Context,
	client *nscale.Client,
	plan NetworkResourceModel,
) (*regionapi.NetworkV2Read, diag.Diagnostics) {
	if diagnostics := setDefaultIDs(client, &plan); diagnostics.HasError() {
		return nil, diagnostics
	}

	params, diagnostics := plan.NscaleNetworkCreateParams(client.OrganizationID)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	createResponse, err := client.Region.PostApiV2Networks(ctx, params)
	if err != nil {
		diagnostics.AddError(
			"Failed to Create Network",
			fmt.Sprintf("An error occurred while creating the network: %s", err),
		)
		return nil, diagnostics
	}
	defer createResponse.Body.Close()

	network, err := nscale.ReadJSONResponsePointer[regionapi.NetworkV2Read](createResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		diagnostics.AddError(
			"Failed to Create Network",
			fmt.Sprintf("An error occurred while creating the network: %s", err),
		)
		return nil, diagnostics
	}

	return network, nil
}

func networkUpdate(
	ctx context.Context,
	client *nscale.Client,
	id string,
	plan NetworkResourceModel,
) (string, diag.Diagnostics) {
	params, diagnostics := plan.NscaleNetworkUpdateParams()
	if diagnostics.HasError() {
		return "", diagnostics
	}

	networkID, err := regionids.ParseNetworkID(id)
	if err != nil {
		diagnostics.AddError(
			"Invalid Network ID",
			fmt.Sprintf("Could not parse network ID %q: %s", id, err),
		)
		return "", diagnostics
	}

	// Tag the update so the watcher can confirm the PUT has propagated through
	// the cache-backed API before reading back a terminal status.
	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	updateResponse, err := client.Region.PutApiV2NetworksNetworkID(ctx, networkID, params)
	if err != nil {
		diagnostics.AddError(
			"Failed to Update Network",
			fmt.Sprintf("An error occurred while updating the network: %s", err),
		)
		return "", diagnostics
	}
	defer updateResponse.Body.Close()

	if _, readErr := nscale.ReadJSONResponsePointer[regionapi.NetworkV2Read](updateResponse); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		diagnostics.AddError(
			"Failed to Update Network",
			fmt.Sprintf("An error occurred while updating the network: %s", readErr),
		)
		return "", diagnostics
	}

	return operationTagKey, nil
}

func networkDelete(ctx context.Context, client *nscale.Client, id string) error {
	networkID, err := regionids.ParseNetworkID(id)
	if err != nil {
		return err
	}

	deleteResponse, err := client.Region.DeleteApiV2NetworksNetworkID(ctx, networkID)
	if err != nil {
		return err
	}
	defer deleteResponse.Body.Close()

	return nscale.ReadEmptyResponse(deleteResponse)
}
