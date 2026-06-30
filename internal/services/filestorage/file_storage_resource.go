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

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	regionids "github.com/unikorn-cloud/region/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/validators"
)

var (
	_ resource.ResourceWithConfigure   = &FileStorageResource{}
	_ resource.ResourceWithImportState = &FileStorageResource{}
)

type FileStorageResourceModel struct {
	FileStorageModel

	RefreshUsage types.Bool       `tfsdk:"refresh_usage"`
	Timeouts     tftimeouts.Value `tfsdk:"timeouts"`
}

type FileStorageResource struct {
	client *nscale.Client
}

func NewFileStorageResource() resource.Resource {
	return &FileStorageResource{}
}

func (r *FileStorageResource) Configure(
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

func (r *FileStorageResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), request, response)
}

func (r *FileStorageResource) Metadata(
	ctx context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_file_storage"
}

func (r *FileStorageResource) Schema(
	ctx context.Context,
	request resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale File Storage",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the file storage.",
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "The amount of storage currently used, in gibibytes.",
				Computed:            true,
			},
			"refresh_usage": schema.BoolAttribute{
				MarkdownDescription: "Whether to refresh the computed `size` usage value from the Nscale API. Set to `false` to keep `size` stable in Terraform state and avoid plan noise from file usage changes.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"capacity": schema.Int64Attribute{
				MarkdownDescription: "The total capacity requested for the file storage, in gibibytes.",
				Required:            true,
			},
			"root_squash": schema.BoolAttribute{
				MarkdownDescription: "Whether root squashing is applied to the file storage to restrict root access for clients.",
				Required:            true,
			},
			"default_snapshot_protection_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether platform-managed Default Snapshot Protection is enabled for the file storage. " +
					"This is separate from any user-managed snapshot policies. When omitted or null, the platform default " +
					"applies and Terraform reads back the resolved value without enforcing it; when set to `true` or `false`, " +
					"Terraform manages the setting and drift-corrects out-of-band changes.",
				Optional: true,
				Computed: true,
			},
			"snapshot_policies": schema.SetNestedAttribute{
				MarkdownDescription: "The user-managed snapshot policies for the file storage. These are separate from " +
					"platform-managed Default Snapshot Protection, which is never represented here. When omitted or null, " +
					"Terraform observes and preserves whatever policies exist remotely; when set to an empty set (`[]`), " +
					"Terraform enforces that no user-managed policies exist; when set to one or more policies, Terraform " +
					"enforces exactly that set. Policies are identified by `name` and ordering is not significant. " +
					"At most four policies are allowed.",
				Optional:   true,
				Computed:   true,
				Validators: snapshotPoliciesSetValidators(),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The snapshot policy name. Acts as the policy's stable identity key and must be unique within the file storage.",
							Required:            true,
							Validators:          snapshotPolicyNameValidators(),
						},
						"schedule": schema.SingleNestedAttribute{
							MarkdownDescription: "When snapshots are taken for this policy.",
							Required:            true,
							Validators:          snapshotScheduleValidators(),
							Attributes: map[string]schema.Attribute{
								"interval": schema.StringAttribute{
									MarkdownDescription: "The snapshot cadence: `hourly`, `daily`, `weekly`, or `monthly`.",
									Required:            true,
									Validators:          snapshotScheduleIntervalValidators(),
								},
								"time_of_day": schema.StringAttribute{
									MarkdownDescription: "The UTC time of day snapshots are taken, in `HH:MMZ` form. Applies to daily, weekly, and monthly schedules.",
									Optional:            true,
									Validators:          snapshotTimeOfDayValidators(),
								},
								"day_of_week": schema.StringAttribute{
									MarkdownDescription: "The UTC day of week snapshots are taken (`monday` through `sunday`). Applies to weekly schedules.",
									Optional:            true,
									Validators:          snapshotDayOfWeekValidators(),
								},
								"day_of_month": schema.Int64Attribute{
									MarkdownDescription: "The UTC day of month snapshots are taken (1 through 28). Applies to monthly schedules.",
									Optional:            true,
									Validators:          snapshotDayOfMonthValidators(),
								},
							},
						},
						"retention": schema.SingleNestedAttribute{
							MarkdownDescription: "How many snapshots this policy retains.",
							Required:            true,
							Attributes: map[string]schema.Attribute{
								"keep": schema.Int64Attribute{
									MarkdownDescription: "The number of snapshots to retain.",
									Required:            true,
									Validators:          snapshotRetentionKeepValidators(),
								},
							},
						},
					},
				},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the file storage.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(validators.NoReservedPrefix(nscale.TerraformOperationTagPrefix)),
				},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the file storage is provisioned. If not specified, this defaults to the project ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the file storage is provisioned. If not specified, this defaults to the region ID configured in the provider.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
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
			"timeouts": tftimeouts.Block(ctx, tftimeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// setDefaults fills attributes that are safe to default on every read/write. The
// project ID is resolved separately at create (see Create) because, unlike these,
// an unresolved project ID must raise an error rather than silently default.
func (r *FileStorageResource) setDefaults(data *FileStorageResourceModel) {
	if data.RegionID.ValueString() == "" {
		data.RegionID = types.StringValue(r.client.RegionID)
	}
	if data.RefreshUsage.IsNull() || data.RefreshUsage.IsUnknown() {
		data.RefreshUsage = types.BoolValue(true)
	}
}

// configuredDefaultSnapshotProtection reads the Default Snapshot Protection
// value exactly as written in configuration. The attribute is optional/computed,
// so its plan and state values can hold a previously API-resolved value; only
// the configuration distinguishes a setting the user explicitly manages (a known
// value, to be enforced) from one they omit (null, to be observed).
func configuredDefaultSnapshotProtection(
	ctx context.Context,
	config tfsdk.Config,
	diagnostics *diag.Diagnostics,
) types.Bool {
	var value types.Bool
	diagnostics.Append(config.GetAttribute(ctx, path.Root("default_snapshot_protection_enabled"), &value)...)
	return value
}

// configuredSnapshotPolicies reads the user-managed Snapshot Policy Set exactly
// as written in configuration. Like Default Snapshot Protection, the attribute
// is optional/computed, so its plan and state values can hold an API-read set;
// only the configuration separates an explicitly managed set — including an
// explicit empty set (`[]`) that enforces no user-managed policies — from an
// omitted/null set that merely observes the remote value.
func configuredSnapshotPolicies(
	ctx context.Context,
	config tfsdk.Config,
	diagnostics *diag.Diagnostics,
) types.Set {
	var value types.Set
	diagnostics.Append(config.GetAttribute(ctx, path.Root("snapshot_policies"), &value)...)
	return value
}

func (m *FileStorageResourceModel) preserveSizeIfUsageRefreshDisabled(previousSize types.Int64) {
	if m.RefreshUsage.ValueBool() {
		return
	}

	m.Size = previousSize
}

func (r *FileStorageResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageResourceModel](ctx, request.Plan.Get, r.setDefaults)
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

	data.DefaultSnapshotProtectionEnabled = configuredDefaultSnapshotProtection(
		ctx,
		request.Config,
		&response.Diagnostics,
	)
	data.SnapshotPolicies = configuredSnapshotPolicies(ctx, request.Config, &response.Diagnostics)
	if response.Diagnostics.HasError() {
		return
	}

	params, diagnostics := data.NscaleFileStorageCreateParams(r.client.OrganizationID)
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
	defer fileStorageCreateResponse.Body.Close()

	fileStorage, err := nscale.ReadJSONResponsePointer[regionapi.StorageV2Read](fileStorageCreateResponse)
	if err != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, err)
		response.Diagnostics.AddError(
			"Failed to Create File Storage",
			fmt.Sprintf("An error occurred while creating the file storage: %s", err),
		)
		return
	}

	data.FileStorageModel = NewFileStorageModel(fileStorage)
	if diagnostics = response.State.Set(ctx, data); diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	stateWatcher := nscale.CreateStateWatcher[regionapi.StorageV2Read]{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context) (*regionapi.StorageV2Read, nscale.ResourceStatus, error) {
			targetID := fileStorage.Metadata.Id
			return nscale.AdaptProjectScoped(getFileStorage(ctx, targetID, r.client))
		},
	}

	fileStorage, ok := stateWatcher.Wait(ctx, data.Timeouts, response)
	if !ok {
		return
	}

	data.FileStorageModel = NewFileStorageModel(fileStorage)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *FileStorageResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageResourceModel](ctx, request.State.Get, r.setDefaults)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}
	previousSize := data.Size

	resourceReader := nscale.ResourceReader[regionapi.StorageV2Read]{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context, id string) (*regionapi.StorageV2Read, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getFileStorage(ctx, id, r.client))
		},
	}

	fileStorage, ok := resourceReader.Read(ctx, data.ID.ValueString(), response)
	if !ok {
		return
	}

	data.FileStorageModel = NewFileStorageModel(fileStorage)
	data.preserveSizeIfUsageRefreshDisabled(previousSize)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *FileStorageResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	priorState, diagnostics := nscale.ReadTerraformState[FileStorageResourceModel](
		ctx,
		request.State.Get,
		r.setDefaults,
	)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	data, diagnostics := nscale.ReadTerraformState[FileStorageResourceModel](ctx, request.Plan.Get, r.setDefaults)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	data.DefaultSnapshotProtectionEnabled = configuredDefaultSnapshotProtection(
		ctx,
		request.Config,
		&response.Diagnostics,
	)
	data.SnapshotPolicies = configuredSnapshotPolicies(ctx, request.Config, &response.Diagnostics)
	if response.Diagnostics.HasError() {
		return
	}

	params, diagnostics := data.NscaleFileStorageUpdateParams()
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	fileStorageID, err := regionids.ParseFileStorageID(id)
	if err != nil {
		response.Diagnostics.AddError(
			"Invalid File Storage ID",
			fmt.Sprintf("Could not parse file storage ID %q: %s", id, err),
		)
		return
	}

	operationTagKey := nscale.WriteOperationTag(&params.Metadata)

	fileStorageUpdateResponse, err := r.client.Region.PutApiV2FilestorageFilestorageID(ctx, fileStorageID, params)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Update File Storage",
			fmt.Sprintf("An error occurred while updating the file storage: %s", err),
		)
		return
	}
	defer fileStorageUpdateResponse.Body.Close()

	if _, readErr := nscale.ReadJSONResponsePointer[regionapi.StorageV2Read](
		fileStorageUpdateResponse,
	); readErr != nil {
		nscale.TerraformDebugLogAPIResponseBody(ctx, readErr)
		response.Diagnostics.AddError(
			"Failed to Update File Storage",
			fmt.Sprintf("An error occurred while updating the file storage: %s", readErr),
		)
		return
	}

	stateWatcher := nscale.UpdateStateWatcher[regionapi.StorageV2Read]{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context) (*regionapi.StorageV2Read, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getFileStorage(ctx, id, r.client))
		},
	}

	fileStorage, ok := stateWatcher.Wait(ctx, operationTagKey, data.Timeouts, response)
	if !ok {
		return
	}

	data.FileStorageModel = NewFileStorageModel(fileStorage)
	data.preserveSizeIfUsageRefreshDisabled(priorState.Size)
	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *FileStorageResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	data, diagnostics := nscale.ReadTerraformState[FileStorageResourceModel](ctx, request.State.Get, r.setDefaults)
	if diagnostics.HasError() {
		response.Diagnostics.Append(diagnostics...)
		return
	}

	id := data.ID.ValueString()

	fileStorageID, err := regionids.ParseFileStorageID(id)
	if err != nil {
		response.Diagnostics.AddError(
			"Invalid File Storage ID",
			fmt.Sprintf("Could not parse file storage ID %q: %s", id, err),
		)
		return
	}

	fileStorageDeleteResponse, err := r.client.Region.DeleteApiV2FilestorageFilestorageID(ctx, fileStorageID)
	if err != nil {
		response.Diagnostics.AddError(
			"Failed to Delete File Storage",
			fmt.Sprintf("An error occurred while deleting the file storage: %s", err),
		)
		return
	}
	defer fileStorageDeleteResponse.Body.Close()

	if err = nscale.ReadEmptyResponse(fileStorageDeleteResponse); err != nil {
		if e, ok := nscale.AsAPIError(err); ok && e.StatusCode != http.StatusNotFound {
			nscale.TerraformDebugLogAPIResponseBody(ctx, err)
			response.Diagnostics.AddError(
				"Failed to Delete File Storage",
				fmt.Sprintf("An error occurred while deleting the file storage: %s", err),
			)
			return
		}
	}

	stateWatcher := nscale.DeleteStateWatcher{
		ResourceTitle: "File Storage",
		ResourceName:  "file storage",
		GetFunc: func(ctx context.Context) (any, nscale.ResourceStatus, error) {
			return nscale.AdaptProjectScoped(getFileStorage(ctx, id, r.client))
		},
	}

	stateWatcher.Wait(ctx, data.Timeouts, response)
}
