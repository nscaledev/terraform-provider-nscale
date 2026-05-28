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

package objectstorage

import (
	"context"

	coreapi "github.com/nscaledev/nscale-sdk-go/common"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

func getObjectStorageEndpoint(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*storageapi.ObjectStorageEndpointRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
	resp, err := client.Storage.GetApiV1ObjectstorageendpointsObjectStorageEndpointID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	endpoint, err := nscale.ReadJSONResponsePointer[storageapi.ObjectStorageEndpointRead](resp)
	if err != nil {
		return nil, nil, err
	}

	return endpoint, &endpoint.Metadata, nil
}

func getObjectStorageAccessKey(
	ctx context.Context,
	endpointID, accessKeyID string,
	client *nscale.Client,
) (*storageapi.ObjectStorageAccessKeyRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
	resp, err := client.Storage.GetApiV1ObjectstorageendpointsObjectStorageEndpointIDAccesskeysObjectStorageAccessKeyID(
		ctx,
		endpointID,
		accessKeyID,
	)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	accessKey, err := nscale.ReadJSONResponsePointer[storageapi.ObjectStorageAccessKeyRead](resp)
	if err != nil {
		return nil, nil, err
	}

	return accessKey, &accessKey.Metadata, nil
}
