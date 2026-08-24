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

	"github.com/google/uuid"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

func getSecurityGroup(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*regionapi.SecurityGroupV2Read, nscale.ResourceStatus, error) {
	securityGroupID, err := uuid.Parse(id)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	securityGroupResponse, err := client.Region.GetApiV2SecuritygroupsSecurityGroupID(ctx, securityGroupID)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	securityGroup, err := nscale.ReadJSONResponsePointer[regionapi.SecurityGroupV2Read](securityGroupResponse)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	return securityGroup, nscale.NewResourceStatus(
		securityGroup.Metadata.Id,
		securityGroup.Metadata.Name,
		securityGroup.Metadata.ProvisioningStatus,
		securityGroup.Metadata.Tags,
	), nil
}
