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

	"github.com/hashicorp/terraform-plugin-framework/diag"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"
	identityapi "github.com/nscaledev/nscale-sdk-go/identity"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"
	storageapi "github.com/nscaledev/nscale-sdk-go/storage"

	// nks is generated in-tree rather than taken from nscale-sdk-go; see
	// internal/nks/gen.go for the reasoning and the migration path.
	"github.com/nscaledev/terraform-provider-nscale/internal/nks"

	// legacycomputeapi is the still-on-unikorn-cloud client used solely by the
	// deprecated nscale_compute_cluster resource. The cluster surface was
	// removed when the compute spec was regenerated for nscale-sdk-go, so
	// until that resource is removed we keep a second client pointed at the
	// legacy spec.
	legacycomputeapi "github.com/unikorn-cloud/compute/pkg/openapi"
)

type Client struct {
	RegionID       string
	OrganizationID string
	ProjectID      string
	Region         regionapi.ClientInterface
	Compute        computeapi.ClientInterface
	Identity       identityapi.ClientInterface
	Reservation    reservationapi.ClientInterface
	LegacyCompute  legacycomputeapi.ClientInterface
	Storage        storageapi.ClientInterface
	// NKS is nil when nks_service_api_endpoint is unset. Reach it through
	// RequireNKS, never directly, so an unconfigured endpoint surfaces as a
	// resource-level diagnostic rather than a nil dereference.
	NKS nks.ClientInterface
}

func NewClient(
	regionServiceBaseURL, computeServiceBaseURL, identityServiceBaseURL, reservationServiceBaseURL, storageServiceBaseURL, nksServiceBaseURL, serviceToken, organizationID, projectID, regionID, userAgent string,
) (*Client, error) {
	httpClient := NewHTTPClient(userAgent, serviceToken)

	region, err := regionapi.NewClient(regionServiceBaseURL, regionapi.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Nscale region API client: %w", err)
	}

	compute, err := computeapi.NewClient(computeServiceBaseURL, computeapi.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Nscale compute API client: %w", err)
	}

	identity, err := identityapi.NewClient(identityServiceBaseURL, identityapi.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Nscale identity API client: %w", err)
	}

	reservation, err := reservationapi.NewClient(
		reservationServiceBaseURL,
		reservationapi.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nscale reservation API client: %w", err)
	}

	legacyCompute, err := legacycomputeapi.NewClient(
		computeServiceBaseURL,
		legacycomputeapi.WithHTTPClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nscale legacy compute API client: %w", err)
	}

	storage, err := storageapi.NewClient(storageServiceBaseURL, storageapi.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Nscale storage API client: %w", err)
	}

	// NKS has no default endpoint yet (the service is pending a migration), so an
	// empty URL is a valid configuration rather than an error. Skip building the
	// client and let RequireNKS explain the omission if an NKS-backed resource is
	// actually used — otherwise every existing configuration would have to set an
	// endpoint it does not need.
	var nksClient nks.ClientInterface
	if nksServiceBaseURL != "" {
		nksClient, err = nks.NewClient(nksServiceBaseURL, nks.WithHTTPClient(httpClient))
		if err != nil {
			return nil, fmt.Errorf("failed to create Nscale NKS API client: %w", err)
		}
	}

	client := &Client{
		RegionID:       regionID,
		OrganizationID: organizationID,
		ProjectID:      projectID,
		Region:         region,
		Compute:        compute,
		Identity:       identity,
		Reservation:    reservation,
		LegacyCompute:  legacyCompute,
		Storage:        storage,
		NKS:            nksClient,
	}

	return client, nil
}

// RequireNKS returns the NKS client, or a diagnostic explaining that the NKS
// endpoint is unconfigured. NKS is the only service without a baked-in default
// URL, so this is the one client that can legitimately be absent at point of
// use; every nscale_kubernetes_* code path must go through here.
// The generated NKS client only exposes an interface (ClientInterface) as its
// mockable surface, and Client stores it as one; returning the concrete type
// here would defeat that.
func (c *Client) RequireNKS() (nks.ClientInterface, diag.Diagnostics) { //nolint:ireturn // the generated client's only public surface is an interface
	var diagnostics diag.Diagnostics

	if c.NKS == nil {
		diagnostics.AddError(
			"Missing NKS Service API Endpoint",
			"The nscale_kubernetes_* resources and data sources require the NKS service "+
				"endpoint to be configured. Set nks_service_api_endpoint on the provider, or "+
				"the NSCALE_NKS_SERVICE_API_ENDPOINT environment variable. Unlike the other "+
				"Nscale services, NKS has no default endpoint yet.",
		)
		return nil, diagnostics
	}

	return c.NKS, diagnostics
}

// ResolveProjectID returns the project ID a project-scoped resource should use:
// the resource's own value when set, otherwise the provider-level default. The
// provider treats project_id as optional at configuration time, so the
// requirement is enforced here, at point of use, with a clear resource-level
// error when neither a resource nor a provider value is available. This lets
// org-level and fully-explicit workflows run without a placeholder project.
func (c *Client) ResolveProjectID(resourceProjectID string) (string, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	switch {
	case resourceProjectID != "":
		return resourceProjectID, diagnostics
	case c.ProjectID != "":
		return c.ProjectID, diagnostics
	default:
		diagnostics.AddError(
			"Missing Project ID",
			"This resource is project-scoped and requires a project ID. Set project_id on the "+
				"resource, or configure a default project_id on the provider (or via the "+
				"NSCALE_PROJECT_ID environment variable).",
		)
		return "", diagnostics
	}
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
