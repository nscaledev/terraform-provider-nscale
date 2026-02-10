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
	"io"
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

type errorResponse struct {
	Error            string  `json:"error"`
	ErrorDescription string  `json:"error_description"`
	TraceID          *string `json:"trace_id"`
}

func ReadJSONResponsePointer[T any](response *http.Response) (*T, error) {
	data, err := ReadJSONResponseValue[T](response)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func ReadJSONResponseValue[T any](response *http.Response) (T, error) {
	var data T

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err := readErrorResponse(response)
		return data, err
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		err = responseReadError(response, err)
		return data, err
	}

	if err = json.Unmarshal(bodyBytes, &data); err != nil {
		err = responseDecodeError(response, bodyBytes, err)
		return data, err
	}

	return data, nil
}

func ReadEmptyResponse(response *http.Response) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return readErrorResponse(response)
	}
	return nil
}

func readErrorResponse(response *http.Response) error {
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return responseReadError(response, err)
	}

	var data errorResponse
	if err = json.Unmarshal(bodyBytes, &data); err != nil {
		return responseDecodeError(response, bodyBytes, err)
	}

	return &APIError{
		StatusCode: response.StatusCode,
		Code:       data.Error,
		Message:    data.ErrorDescription,
		TraceID:    data.TraceID,
	}
}

func responseReadError(response *http.Response, err error) error {
	return &APIError{
		StatusCode: response.StatusCode,
		Message:    fmt.Sprintf("failed to read response body: %s", err),
	}
}

func responseDecodeError(response *http.Response, bodyBytes []byte, err error) error {
	var endpoint string
	if response.Request != nil {
		endpoint = fmt.Sprintf("%s %s", response.Request.Method, response.Request.URL.Path)
	}

	return &APIError{
		StatusCode: response.StatusCode,
		Message:    fmt.Sprintf("failed to decode response: %s", err),
		Endpoint:   endpoint,
		BodyBytes:  bodyBytes,
	}
}
