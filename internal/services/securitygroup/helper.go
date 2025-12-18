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
	"net/http"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

func getSecurityGroup(ctx context.Context, id string, client *nscale.Client) (*regionapi.SecurityGroupV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
	securityGroupResponse, err := client.Region.GetApiV2SecuritygroupsSecurityGroupIDWithResponse(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	if securityGroupResponse.StatusCode() != http.StatusOK {
		err = nscale.NewStatusCodeError(securityGroupResponse.StatusCode())
		return nil, nil, err
	}

	securityGroup := securityGroupResponse.JSON200
	if securityGroup == nil {
		return nil, nil, nscale.ErrEmptyResponse
	}

	return securityGroup, &securityGroup.Metadata, nil
}
