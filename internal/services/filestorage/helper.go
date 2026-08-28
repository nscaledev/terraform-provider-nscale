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

	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	regionids "github.com/unikorn-cloud/region/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

func getFileStorage(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*regionapi.StorageV2Read, nscale.ResourceStatus, error) {
	fileStorageID, err := regionids.ParseFileStorageID(id)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	fileStorageResponse, err := client.Region.GetApiV2FilestorageFilestorageID(
		ctx,
		regionapi.FilestorageIDParameter(fileStorageID),
	)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}
	defer fileStorageResponse.Body.Close()

	fileStorage, err := nscale.ReadJSONResponsePointer[regionapi.StorageV2Read](fileStorageResponse)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	return fileStorage, nscale.NewResourceStatus(
		fileStorage.Metadata.Id,
		fileStorage.Metadata.Name,
		string(fileStorage.Metadata.ProvisioningStatus),
		fileStorage.Metadata.Tags,
	), nil
}
