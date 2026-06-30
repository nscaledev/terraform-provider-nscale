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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

var _ datasource.DataSourceWithConfigure = &FileStorageDataSource{}

// FileStorageDataSource embeds the generic read+map base; only Schema and the
// adapter wiring below are file-storage-specific.
type FileStorageDataSource struct {
	*nscale.GenericDataSource[FileStorageModel, regionapi.StorageV2Read]
}

func NewFileStorageDataSource() datasource.DataSource {
	return &FileStorageDataSource{
		GenericDataSource: nscale.NewGenericDataSource(
			nscale.DataSourceAdapter[FileStorageModel, regionapi.StorageV2Read]{
				TypeNameSuffix: "_file_storage",
				Title:          "File Storage",
				Name:           "file storage",
				Get: func(ctx context.Context, client *nscale.Client, id string) (*regionapi.StorageV2Read, error) {
					fs, _, err := getFileStorage(ctx, id, client)
					return fs, err
				},
				ToModel:     NewFileStorageModel,
				IDFromModel: func(m FileStorageModel) string { return m.ID.ValueString() },
			},
		),
	}
}

func (s *FileStorageDataSource) Schema(
	ctx context.Context,
	request datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		MarkdownDescription: "Nscale File Storage",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "A unique identifier for the file storage.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the file storage.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "The description of the file storage.",
				Computed:            true,
			},
			"storage_class_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the storage class assigned to the file storage.",
				Computed:            true,
			},
			"size": schema.Int64Attribute{
				MarkdownDescription: "The amount of storage currently used, in gibibytes.",
				Computed:            true,
			},
			"capacity": schema.Int64Attribute{
				MarkdownDescription: "The total capacity of the file storage, in gibibytes.",
				Computed:            true,
			},
			"root_squash": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether root squashing is enabled for the file storage.",
				Computed:            true,
			},
			"default_snapshot_protection_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether platform-managed Default Snapshot Protection is enabled for the file storage. " +
					"This is separate from any user-managed snapshot policies.",
				Computed: true,
			},
			"snapshot_policies": schema.SetNestedAttribute{
				MarkdownDescription: "The user-managed snapshot policies for the file storage, identified by `name`.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "The snapshot policy name.",
							Computed:            true,
						},
						"schedule": schema.SingleNestedAttribute{
							MarkdownDescription: "When snapshots are taken for this policy.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"interval": schema.StringAttribute{
									MarkdownDescription: "The snapshot cadence: `hourly`, `daily`, `weekly`, or `monthly`.",
									Computed:            true,
								},
								"time_of_day": schema.StringAttribute{
									MarkdownDescription: "The UTC time of day snapshots are taken, in `HH:MMZ` form.",
									Computed:            true,
								},
								"day_of_week": schema.StringAttribute{
									MarkdownDescription: "The day of week snapshots are taken (`monday` through `sunday`).",
									Computed:            true,
								},
								"day_of_month": schema.Int64Attribute{
									MarkdownDescription: "The day of month snapshots are taken (1 through 28).",
									Computed:            true,
								},
							},
						},
						"retention": schema.SingleNestedAttribute{
							MarkdownDescription: "How many snapshots this policy retains.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"keep": schema.Int64Attribute{
									MarkdownDescription: "The number of snapshots to retain.",
									Computed:            true,
								},
							},
						},
					},
				},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "A map of tags assigned to the file storage.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the project where the file storage is provisioned.",
				Computed:            true,
			},
			"region_id": schema.StringAttribute{
				MarkdownDescription: "The identifier of the region where the file storage is provisioned.",
				Computed:            true,
			},
			"creation_time": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the file storage was created.",
				Computed:            true,
			},
		},
		Blocks: map[string]schema.Block{
			"network": schema.ListNestedBlock{
				MarkdownDescription: "The network to which the file storage is attached.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "The identifier of the network to which the file storage is attached.",
							Computed:            true,
						},
						"mount_source": schema.StringAttribute{
							MarkdownDescription: "The network path for mounting the file storage.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}
