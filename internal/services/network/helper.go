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

package network

import (
	"context"
	"net/http"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

func getNetwork(ctx context.Context, id string, client *nscale.Client) (*regionapi.NetworkV2Read, *coreapi.ProjectScopedResourceReadMetadata, error) {
	networkResponse, err := client.Region.GetApiV2NetworksNetworkIDWithResponse(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	if networkResponse.StatusCode() != http.StatusOK {
		err = nscale.NewStatusCodeError(networkResponse.StatusCode())
		return nil, nil, err
	}

	network := networkResponse.JSON200
	if network == nil {
		return nil, nil, nscale.ErrEmptyResponse
	}

	return network, &network.Metadata, nil
}
