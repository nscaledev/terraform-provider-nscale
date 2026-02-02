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

package instance

import (
	"context"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

func getInstance(ctx context.Context, id string, client *nscale.Client) (*computeapi.InstanceRead, *coreapi.ProjectScopedResourceReadMetadata, error) {
	instanceResponse, err := client.Compute.GetApiV2InstancesInstanceID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	instance, err := nscale.ReadJSONResponsePointer[computeapi.InstanceRead](instanceResponse)
	if err != nil {
		return nil, nil, err
	}

	return instance, &instance.Metadata, nil
}
