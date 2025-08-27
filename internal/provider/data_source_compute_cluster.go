package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	nscale "github.com/nscaledev/terraform-provider-nscale/internal/client"
)

var _ datasource.DataSourceWithConfigure = &ComputeClusterDataSource{}

type ComputeClusterDataSource struct {
	client         *nscale.ClientWithResponses
	organizationID string
	projectID      string
}

func NewComputeClusterDataSource() datasource.DataSource {
	return &ComputeClusterDataSource{}
}

func (s *ComputeClusterDataSource) Configure(ctx context.Context, request datasource.ConfigureRequest, response *datasource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	config, ok := request.ProviderData.(*NscaleProviderConfig)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *NscaleProviderConfig, got: %T. Please contact the Nscale team for support.", request.ProviderData),
		)
		return
	}

	s.client = config.client
	s.organizationID = config.organizationID
	s.projectID = config.projectID
}

func (s *ComputeClusterDataSource) Metadata(ctx context.Context, request datasource.MetadataRequest, response *datasource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_compute_cluster"
}

func (s *ComputeClusterDataSource) Schema(ctx context.Context, request datasource.SchemaRequest, response *datasource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale Compute Cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "An unique identifier for the compute cluster.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the compute cluster.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "A description of the compute cluster.",
				Computed:            true,
			},
			"workload_pools": schema.ListNestedAttribute{
				MarkdownDescription: "A list of pools of workload nodes in the compute cluster.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The name of the workload pool.",
							Computed:            true,
						},
						"replicas": schema.Int64Attribute{
							MarkdownDescription: "The number of replicas (VMs) to provision in this workload pool.",
							Computed:            true,
						},
						"image_id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the image used for initializing the boot disk of the workload pool VMs.",
							Computed:            true,
						},
						"flavor_id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the flavor (machine type) used for the workload pool VMs.",
							Computed:            true,
						},
						//"disk_size": schema.Int64Attribute{
						//	MarkdownDescription: "The size of the boot disk for each VM in the workload pool, in GiB.",
						//	Computed:            true,
						//},
						"user_data": schema.StringAttribute{
							MarkdownDescription: "The data to pass to the VMs at boot time.",
							Computed:            true,
						},
						"enable_public_ip": schema.BoolAttribute{
							MarkdownDescription: "Whether to assign a public IP address to each VM in this workload pool.",
							Computed:            true,
						},
						"firewall_rules": schema.ListNestedAttribute{
							MarkdownDescription: "A list of firewall rules applied to the VMs in this workload pool.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"direction": schema.StringAttribute{
										MarkdownDescription: "The direction of the traffic to which this firewall rule applies.",
										Computed:            true,
									},
									"protocol": schema.StringAttribute{
										MarkdownDescription: "The IP protocol to which this firewall rule applies.",
										Computed:            true,
									},
									"ports": schema.StringAttribute{
										MarkdownDescription: "The ports to which this firewall rule applies. This can be a single port, or a range of ports.",
										Computed:            true,
									},
									"prefixes": schema.SetAttribute{
										MarkdownDescription: "A set of CIDR prefixes to which this firewall rule applies.",
										ElementType:         types.StringType,
										Computed:            true,
									},
								},
							},
						},
						"machines": schema.ListNestedAttribute{
							MarkdownDescription: "A list of machines in this workload pool.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"hostname": schema.StringAttribute{
										MarkdownDescription: "The hostname of the machine.",
										Computed:            true,
									},
									"private_ip": schema.StringAttribute{
										MarkdownDescription: "The private IP address of the machine.",
										Computed:            true,
									},
									"public_ip": schema.StringAttribute{
										MarkdownDescription: "The public IP address of the machine, if assigned.",
										Computed:            true,
									},
								},
							},
						},
					},
				},
			},
			"ssh_private_key": schema.StringAttribute{
				MarkdownDescription: "The SSH private key for accessing the compute cluster.",
				Computed:            true,
				Sensitive:           true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the compute cluster is provisioned.",
				Computed:            true,
			},
			"provisioning_status": schema.StringAttribute{
				MarkdownDescription: "The provisioning status of the compute cluster.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the compute cluster was created.",
				Computed:            true,
			},
		},
	}
}

func (s *ComputeClusterDataSource) Read(ctx context.Context, request datasource.ReadRequest, response *datasource.ReadResponse) {
	var data ComputeClusterModel

	response.Diagnostics.Append(request.Config.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	clusterListResponse, err := s.client.GetApiV1OrganizationsOrganizationIDClustersWithResponse(ctx, s.organizationID, nil)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Read Compute Cluster",
			fmt.Sprintf("An error occurred while reading the compute cluster: %s", err),
		)
		return
	}
	if clusterListResponse.StatusCode() != http.StatusOK || clusterListResponse.JSON200 == nil {
		response.Diagnostics.AddError(
			"Failed to Read Compute Cluster",
			fmt.Sprintf("The compute cluster read operation failed with status code %d.", clusterListResponse.StatusCode()),
		)
		return
	}

	for _, cluster := range *clusterListResponse.JSON200 {
		if cluster.Metadata.Id == data.ID.ValueString() {
			data = NewComputeClusterModel(&cluster)
			response.Diagnostics.Append(response.State.Set(ctx, data)...)
			return
		}
	}
}
