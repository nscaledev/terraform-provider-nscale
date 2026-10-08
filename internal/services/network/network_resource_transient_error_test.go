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

package network_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	regionapi "github.com/nscaledev/nscale-sdk-go/region"
)

const (
	mockNetworkID      = "5b0e7c1e-3f4a-4c2b-9d8e-1a2b3c4d5e6f"
	mockOrganizationID = "0f1e2d3c-4b5a-4968-8776-655443322110"
	mockProjectID      = "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	mockRegionID       = "9d8c7b6a-5f4e-4d3c-2b1a-0f9e8d7c6b5a"
)

// TestAccNetworkResource_transientErrorDuringCreate reproduces DX-2622 against
// a mock region API: the network reports 'error' partway through create, then
// recovers to 'provisioned'. The apply must succeed without tainting the
// resource, so the post-apply plan is empty and no second network is created.
//
// The mock stands in for staging because the transient error only happens in
// regions with a slow backend network create, which a live run can't force.
func TestAccNetworkResource_transientErrorDuringCreate(t *testing.T) {
	api := newMockNetworkAPI(t, []regionapi.ResourceProvisioningStatus{
		regionapi.ResourceProvisioningStatusProvisioning,
		regionapi.ResourceProvisioningStatusError,
		regionapi.ResourceProvisioningStatusError,
		regionapi.ResourceProvisioningStatusProvisioned,
	})

	// Environment variables take precedence over provider config, so point the
	// provider at the mock this way, overriding any live credentials.
	t.Setenv("NSCALE_REGION_SERVICE_API_ENDPOINT", api.server.URL)
	t.Setenv("NSCALE_SERVICE_TOKEN", "mock-token")
	t.Setenv("NSCALE_ORGANIZATION_ID", mockOrganizationID)
	t.Setenv("NSCALE_PROJECT_ID", mockProjectID)
	t.Setenv("NSCALE_REGION_ID", mockRegionID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if !api.deleted() {
				return fmt.Errorf("network %s was not deleted", mockNetworkID)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: testAccNetworkResourceConfig("tf-acc-network-transient", "192.168.240.0/24"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nscale_network.test", "id", mockNetworkID),
					func(*terraform.State) error {
						if !api.sawError() {
							return errors.New("mock never reported 'error', so the transient path was not exercised")
						}
						return nil
					},
				),
			},
			{
				// A tainted resource would plan a replacement here.
				Config:   testAccNetworkResourceConfig("tf-acc-network-transient", "192.168.240.0/24"),
				PlanOnly: true,
			},
		},
	})

	if got := api.creates(); got != 1 {
		t.Fatalf("network create requests = %d, want 1", got)
	}
}

// mockNetworkAPI serves the region API endpoints nscale_network uses. GETs walk
// through statuses, then stay on the last one until the network is deleted.
type mockNetworkAPI struct {
	server *httptest.Server

	mu          sync.Mutex
	statuses    []regionapi.ResourceProvisioningStatus
	reads       int
	createCalls int
	errorSeen   bool
	isDeleted   bool
	network     regionapi.NetworkV2Read
}

func newMockNetworkAPI(t *testing.T, statuses []regionapi.ResourceProvisioningStatus) *mockNetworkAPI {
	t.Helper()

	api := &mockNetworkAPI{statuses: statuses}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/networks", api.handleCreate)
	mux.HandleFunc("GET /api/v2/networks/{id}", api.handleGet)
	mux.HandleFunc("DELETE /api/v2/networks/{id}", api.handleDelete)

	api.server = httptest.NewServer(mux)
	t.Cleanup(api.server.Close)

	return api
}

func (a *mockNetworkAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
	var request regionapi.NetworkV2Create
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeMockError(w, http.StatusBadRequest, err.Error())
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.createCalls++
	a.network = regionapi.NetworkV2Read{
		Metadata: regionapi.ProjectScopedResourceReadMetadata{
			Id:                 mockNetworkID,
			Name:               request.Metadata.Name,
			Description:        request.Metadata.Description,
			Tags:               request.Metadata.Tags,
			OrganizationId:     request.Spec.OrganizationId,
			ProjectId:          request.Spec.ProjectId,
			CreationTime:       time.Now().UTC().Truncate(time.Second),
			HealthStatus:       regionapi.ResourceHealthStatusUnknown,
			ProvisioningStatus: regionapi.ResourceProvisioningStatusPending,
		},
		Spec: regionapi.NetworkV2Spec{
			DnsNameservers: request.Spec.DnsNameservers,
			Routes:         request.Spec.Routes,
		},
		Status: regionapi.NetworkV2Status{
			Prefix:   request.Spec.Prefix,
			RegionId: request.Spec.RegionId.String(),
		},
	}

	writeMockJSON(w, http.StatusAccepted, a.network)
}

func (a *mockNetworkAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.createCalls == 0 || a.isDeleted || r.PathValue("id") != mockNetworkID {
		writeMockError(w, http.StatusNotFound, "network not found")
		return
	}

	status := a.statuses[min(a.reads, len(a.statuses)-1)]
	a.reads++

	if status == regionapi.ResourceProvisioningStatusError {
		a.errorSeen = true
	}

	a.network.Metadata.ProvisioningStatus = status
	writeMockJSON(w, http.StatusOK, a.network)
}

func (a *mockNetworkAPI) handleDelete(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.createCalls == 0 || a.isDeleted || r.PathValue("id") != mockNetworkID {
		writeMockError(w, http.StatusNotFound, "network not found")
		return
	}

	a.isDeleted = true
	w.WriteHeader(http.StatusAccepted)
}

func (a *mockNetworkAPI) creates() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.createCalls
}

func (a *mockNetworkAPI) sawError() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.errorSeen
}

func (a *mockNetworkAPI) deleted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isDeleted
}

func writeMockJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeMockError(w http.ResponseWriter, status int, description string) {
	writeMockJSON(w, status, map[string]string{"error": http.StatusText(status), "error_description": description})
}
