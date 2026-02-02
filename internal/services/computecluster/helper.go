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

package computecluster

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

func getComputeCluster(ctx context.Context, organizationID, id string, client *nscale.Client) (*computeapi.ComputeClusterRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
	computeClusterListResponse, err := client.Compute.GetApiV1OrganizationsOrganizationIDClusters(ctx, organizationID, nil)
	if err != nil {
		return nil, nil, err
	}

	computeClusters, err := nscale.ReadJSONResponseValue[[]computeapi.ComputeClusterRead](computeClusterListResponse)
	if err != nil {
		return nil, nil, err
	}

	for _, computeCluster := range computeClusters {
		if computeCluster.Metadata.Id == id {
			return &computeCluster, &computeCluster.Metadata, nil
		}
	}

	err = &nscale.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("failed to find compute cluster '%s' in the list response", id),
	}

	return nil, nil, err
}
