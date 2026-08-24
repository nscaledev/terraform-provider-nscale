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

	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	regionids "github.com/unikorn-cloud/region/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// getNetwork returns the network alongside the ResourceStatus the shared
// watchers consume. Projecting the metadata here, where the region package's
// concrete type is still in scope, keeps that type out of the shared layer.
func getNetwork(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*regionapi.NetworkV2Read, nscale.ResourceStatus, error) {
	networkID, err := regionids.ParseNetworkID(id)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	networkResponse, err := client.Region.GetApiV2NetworksNetworkID(ctx, networkID)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}
	defer networkResponse.Body.Close()

	network, err := nscale.ReadJSONResponsePointer[regionapi.NetworkV2Read](networkResponse)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	metadata := network.Metadata

	return network, nscale.NewResourceStatus(
		metadata.Id,
		metadata.Name,
		metadata.ProvisioningStatus,
		metadata.Tags,
	), nil
}
