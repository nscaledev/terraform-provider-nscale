/*
Copyright 2025 Nscale

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

package securitygroup

import (
	"context"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

func getSecurityGroup(ctx context.Context, id string, client *nscale.Client) (*regionapi.SecurityGroupV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
	securityGroupResponse, err := client.Region.GetApiV2SecuritygroupsSecurityGroupID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	securityGroup, err := nscale.ReadJSONResponsePointer[regionapi.SecurityGroupV2Read](securityGroupResponse)
	if err != nil {
		return nil, nil, err
	}

	return securityGroup, &securityGroup.Metadata, nil
}

func findInstancesUsingSecurityGroup(ctx context.Context, client *nscale.Client, networkID, securityGroupID string) ([]string, error) {
	organizationFilter := computeapi.OrganizationIDQueryParameter{client.OrganizationID}
	projectFilter := computeapi.ProjectIDQueryParameter{client.ProjectID}
	regionFilter := computeapi.RegionIDQueryParameter{client.RegionID}

	params := &computeapi.GetApiV2InstancesParams{
		OrganizationID: &organizationFilter,
		ProjectID:      &projectFilter,
		RegionID:       &regionFilter,
	}
	if networkID != "" {
		networkFilter := computeapi.NetworkIDQueryParameter{networkID}
		params.NetworkID = &networkFilter
	}

	response, err := client.Compute.GetApiV2Instances(ctx, params)
	if err != nil {
		return nil, err
	}

	instances, err := nscale.ReadJSONResponseValue[computeapi.InstancesRead](response)
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, instance := range instances {
		if instance.Spec.Networking == nil || instance.Spec.Networking.SecurityGroups == nil {
			continue
		}
		for _, sgID := range *instance.Spec.Networking.SecurityGroups {
			if sgID == securityGroupID {
				matches = append(matches, instance.Metadata.Id)
				break
			}
		}
	}

	return matches, nil
}
