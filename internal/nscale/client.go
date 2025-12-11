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

package nscale

import (
	"fmt"

	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type Client struct {
	OrganizationID string
	ProjectID      string
	Region         regionapi.ClientWithResponsesInterface
	Compute        computeapi.ClientWithResponsesInterface
}

func NewClient(regionServiceBaseURL, computeServiceBaseURL, serviceToken, organizationID, projectID, userAgent string) (*Client, error) {
	httpClient := NewHTTPClient(userAgent, serviceToken)

	region, err := regionapi.NewClientWithResponses(regionServiceBaseURL, regionapi.WithHTTPClient(httpClient))
	if err != nil {
		err = fmt.Errorf("failed to create Nscale region API client: %w", err)
		return nil, err
	}

	compute, err := computeapi.NewClientWithResponses(computeServiceBaseURL, computeapi.WithHTTPClient(httpClient))
	if err != nil {
		err = fmt.Errorf("failed to create Nscale compute API client: %w", err)
		return nil, err
	}

	client := &Client{
		OrganizationID: organizationID,
		ProjectID:      projectID,
		Region:         region,
		Compute:        compute,
	}

	return client, nil
}
