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
	"encoding/json"
	"fmt"
	"net/http"

	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type Client struct {
	RegionID       string
	OrganizationID string
	ProjectID      string
	Region         regionapi.ClientInterface
	Compute        computeapi.ClientInterface
}

func NewClient(regionServiceBaseURL, computeServiceBaseURL, serviceToken, organizationID, projectID, regionID, userAgent string) (*Client, error) {
	httpClient := NewHTTPClient(userAgent, serviceToken)

	region, err := regionapi.NewClient(regionServiceBaseURL, regionapi.WithHTTPClient(httpClient))
	if err != nil {
		err = fmt.Errorf("failed to create Nscale region API client: %w", err)
		return nil, err
	}

	compute, err := computeapi.NewClient(computeServiceBaseURL, computeapi.WithHTTPClient(httpClient))
	if err != nil {
		err = fmt.Errorf("failed to create Nscale compute API client: %w", err)
		return nil, err
	}

	client := &Client{
		RegionID:       regionID,
		OrganizationID: organizationID,
		ProjectID:      projectID,
		Region:         region,
		Compute:        compute,
	}

	return client, nil
}

type StatusCodeMatcher func(statusCode int) bool

func StatusCodeAny(statusCodes ...int) StatusCodeMatcher {
	return func(statusCode int) bool {
        return slices.Contains(statusCodes, statusCode)
	}
}

type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func ReadJSONResponsePointer[T any](response *http.Response, statusCodeMatcher StatusCodeMatcher) (*T, error) {
	data, err := ReadJSONResponseValue[T](response, statusCodeMatcher)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func ReadJSONResponseValue[T any](response *http.Response, statusCodeMatcher StatusCodeMatcher) (T, error) {
	var data T

	if !statusCodeMatcher(response.StatusCode) {
		err := readErrorResponse(response)
		return data, err
	}

	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		err = responseDecodeError(response, err)
		return data, err
	}

	return data, nil
}

func ReadErrorResponse(response *http.Response, statusCodeMatcher StatusCodeMatcher) error {
	if !statusCodeMatcher(response.StatusCode) {
		return readErrorResponse(response)
	}
	return nil
}

func readErrorResponse(response *http.Response) error {
	var data errorResponse
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return responseDecodeError(response, err)
	}

	return &APIError{
		StatusCode: response.StatusCode,
		Code:       data.Error,
		Message:    data.ErrorDescription,
	}
}

func responseDecodeError(response *http.Response, err error) error {
	return &APIError{
		StatusCode: response.StatusCode,
		Message:    fmt.Sprintf("failed to decode response: %s", err),
	}
}
