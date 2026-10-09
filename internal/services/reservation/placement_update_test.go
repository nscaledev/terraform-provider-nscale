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

package reservation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	reservationapi "github.com/nscaledev/nscale-sdk-go/reservation"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

const (
	updateTestImageID    = "5c9b2e7f-1a4d-4b8c-8f2e-3d6a9b0c1e52"
	updateTestNewImageID = "2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40"
)

func updateTestPlacement() *reservationapi.PlacementV2Read {
	userData := []byte("#!/bin/sh\n")

	return &reservationapi.PlacementV2Read{
		Metadata: reservationapi.ProjectScopedResourceReadMetadata{
			Id:                 "placement-1",
			Name:               "training-workers",
			Description:        new("host placement"),
			ProjectId:          "project-1",
			ProvisioningStatus: reservationapi.ResourceProvisioningStatusProvisioned,
			Tags:               &[]reservationapi.Tag{{Name: "workload", Value: "training"}},
		},
		Spec: reservationapi.PlacementV2Spec{
			Count: 4,
			Constraints: reservationapi.PlacementConstraintsV2{
				Policy:  reservationapi.PlacementPolicyV2Spread,
				MaxSkew: new(1),
			},
			ReadinessPolicy: &reservationapi.PlacementReadinessPolicyV2{
				Mode: reservationapi.PlacementReadinessModeV2Prefer,
			},
			ServerSpec: reservationapi.PlacementServerSpecV2{
				ImageId:                   updateTestImageID,
				SshCertificateAuthorityId: new("ca-1"),
				UserData:                  &userData,
				Networking: &reservationapi.PlacementServerNetworkingV2{
					PublicIP: new(true),
				},
			},
			UpdateStrategy: &reservationapi.PlacementUpdateStrategyV2{
				Type: reservationapi.PlacementUpdateStrategyTypeV2Manual,
			},
		},
		Status: reservationapi.PlacementV2Status{
			RegionId:         "region-1",
			ReservationId:    "reservation-1",
			NetworkId:        "network-1",
			StatusCurrent:    true,
			UpdatedHostCount: new(4),
			DriftedHostCount: new(0),
		},
	}
}

func TestNscalePlacementUpdateParamsKeepsReadSpec(t *testing.T) {
	current := updateTestPlacement()

	planned := updateTestPlacement()
	planned.Spec.ServerSpec.ImageId = updateTestNewImageID
	model := NewPlacementModel(planned)

	params, diagnostics := model.NscalePlacementUpdateParams(t.Context(), current)
	if diagnostics.HasError() {
		t.Fatalf("NscalePlacementUpdateParams() diagnostics = %v", diagnostics)
	}

	if params.Spec.ServerSpec.ImageId != updateTestNewImageID {
		t.Errorf("ImageId = %q, want %q", params.Spec.ServerSpec.ImageId, updateTestNewImageID)
	}

	want := updateTestPlacement().Spec
	want.ServerSpec.ImageId = updateTestNewImageID

	if !reflect.DeepEqual(params.Spec, want) {
		t.Errorf("Spec = %+v, want the read spec with only the image changed: %+v", params.Spec, want)
	}

	body, err := json.Marshal(params.Spec.ServerSpec.Networking)
	if err != nil {
		t.Fatalf("marshal networking: %v", err)
	}

	if got := string(body); got != `{"publicIP":true}` {
		t.Errorf("networking = %s, want the unset lists omitted", got)
	}

	if current.Spec.ServerSpec.ImageId != updateTestImageID {
		t.Errorf("current ImageId = %q, want the read left unchanged", current.Spec.ServerSpec.ImageId)
	}

	if params.Metadata.Name != current.Metadata.Name {
		t.Errorf("Metadata.Name = %q, want %q", params.Metadata.Name, current.Metadata.Name)
	}
}

func TestNscalePlacementUpdateParamsUpdateStrategy(t *testing.T) {
	rolling := &reservationapi.PlacementUpdateStrategyV2{
		Type: reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate,
		RollingUpdate: &reservationapi.PlacementRollingUpdateV2{
			MaxUnavailable: new("2"),
		},
	}

	planned := updateTestPlacement()
	planned.Spec.UpdateStrategy = rolling
	model := NewPlacementModel(planned)

	params, diagnostics := model.NscalePlacementUpdateParams(t.Context(), updateTestPlacement())
	if diagnostics.HasError() {
		t.Fatalf("NscalePlacementUpdateParams() diagnostics = %v", diagnostics)
	}

	if !reflect.DeepEqual(params.Spec.UpdateStrategy, rolling) {
		t.Errorf("UpdateStrategy = %+v, want %+v", params.Spec.UpdateStrategy, rolling)
	}

	current := updateTestPlacement()
	current.Spec.UpdateStrategy = rolling

	model.UpdateStrategy = newPlacementUpdateStrategyObject(nil)

	params, diagnostics = model.NscalePlacementUpdateParams(t.Context(), current)
	if diagnostics.HasError() {
		t.Fatalf("NscalePlacementUpdateParams() diagnostics = %v", diagnostics)
	}

	if !reflect.DeepEqual(params.Spec.UpdateStrategy, rolling) {
		t.Errorf("UpdateStrategy = %+v, want the read %+v", params.Spec.UpdateStrategy, rolling)
	}
}

func rollingUpdateStrategy() *reservationapi.PlacementUpdateStrategyV2 {
	return &reservationapi.PlacementUpdateStrategyV2{
		Type: reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate,
	}
}

func TestPlacementUpdateProgress(t *testing.T) {
	testCases := []struct {
		name     string
		mutate   func(*reservationapi.PlacementV2Read)
		wantDone bool
		wantErr  error
	}{
		{
			name:   "manual waits for the service to observe the update",
			mutate: func(p *reservationapi.PlacementV2Read) { p.Status.StatusCurrent = false },
		},
		{
			name: "manual is done once observed, with servers left drifted",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Status.ReadyHostCount = new(4)
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(4)
			},
			wantDone: true,
		},
		{
			name: "an absent strategy is manual",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = nil
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(4)
			},
			wantDone: true,
		},
		{
			name: "manual waits when a reconcile zeroed the counts without observing servers",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusProvisioning
				p.Status.ReadyHostCount = new(0)
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(0)
				p.Status.InFlightCount = new(0)
				p.Status.StalledCount = new(0)
			},
		},
		{
			name: "manual waits while the placement still reads provisioning",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusProvisioning
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(4)
			},
		},
		{
			name: "manual waits until every server is counted",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(3)
			},
		},
		{
			name: "manual fails when errored",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusError
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(4)
			},
			wantErr: errPlacementUpdateErrored,
		},
		{
			name: "an error before the update is observed fails",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Status.StatusCurrent = false
				p.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusError
			},
			wantErr: errPlacementUpdateErrored,
		},
		{
			name: "rolling ignores counts the service has not refreshed",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Status.StatusCurrent = false
				p.Status.DriftedHostCount = new(0)
				p.Status.InFlightCount = new(0)
			},
		},
		{
			name: "rolling waits while servers are drifted",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Status.DriftedHostCount = new(3)
				p.Status.InFlightCount = new(0)
			},
		},
		{
			name: "rolling waits while servers are in flight",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Status.DriftedHostCount = new(0)
				p.Status.InFlightCount = new(1)
			},
		},
		{
			name: "rolling is done when every server has converged",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Status.UpdatedHostCount = new(4)
				p.Status.DriftedHostCount = new(0)
				p.Status.InFlightCount = new(0)
			},
			wantDone: true,
		},
		{
			name: "rolling waits when a reconcile zeroed the counts without observing servers",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusProvisioning
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(0)
				p.Status.InFlightCount = new(0)
				p.Status.StalledCount = new(0)
			},
		},
		{
			name: "rolling waits while the placement still reads provisioning",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusProvisioning
				p.Status.UpdatedHostCount = new(4)
				p.Status.DriftedHostCount = new(0)
				p.Status.InFlightCount = new(0)
			},
		},
		{
			name: "rolling waits until every server is updated",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Status.UpdatedHostCount = new(3)
				p.Status.DriftedHostCount = new(0)
				p.Status.InFlightCount = new(0)
			},
		},
		{
			name: "rolling fails on a stalled server",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Status.DriftedHostCount = new(2)
				p.Status.InFlightCount = new(1)
				p.Status.StalledCount = new(1)
			},
			wantErr: errPlacementUpdateStalled,
		},
		{
			name: "rolling fails when errored",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = rollingUpdateStrategy()
				p.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusError
				p.Status.DriftedHostCount = new(2)
				p.Status.InFlightCount = new(0)
			},
			wantErr: errPlacementUpdateErrored,
		},
		{
			name: "an unknown strategy fails rather than completing by the manual rule",
			mutate: func(p *reservationapi.PlacementV2Read) {
				p.Spec.UpdateStrategy = &reservationapi.PlacementUpdateStrategyV2{Type: "Recreate"}
				p.Status.ReadyHostCount = new(4)
				p.Status.UpdatedHostCount = new(0)
				p.Status.DriftedHostCount = new(4)
			},
			wantErr: errPlacementUpdateStrategyUnknown,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			placement := updateTestPlacement()
			testCase.mutate(placement)

			done, err := placementUpdateProgress(placement)

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("placementUpdateProgress() error = %v, want %v", err, testCase.wantErr)
			}

			if done != testCase.wantDone {
				t.Errorf("placementUpdateProgress() done = %t, want %t", done, testCase.wantDone)
			}
		})
	}
}

func setPlacementFailureGrace(t *testing.T, grace time.Duration) {
	t.Helper()

	restore := placementFailureGrace
	placementFailureGrace = grace

	t.Cleanup(func() { placementFailureGrace = restore })
}

func reads(
	placements ...*reservationapi.PlacementV2Read,
) func(context.Context) (*reservationapi.PlacementV2Read, error) {
	calls := 0

	return func(context.Context) (*reservationapi.PlacementV2Read, error) {
		placement := placements[min(calls, len(placements)-1)]
		calls++

		return placement, nil
	}
}

func unobserved() *reservationapi.PlacementV2Read {
	placement := updateTestPlacement()
	placement.Status.StatusCurrent = false

	return placement
}

func erroredBeforeObserved() *reservationapi.PlacementV2Read {
	placement := updateTestPlacement()
	placement.Status.StatusCurrent = false
	placement.Metadata.ProvisioningStatus = reservationapi.ResourceProvisioningStatusError
	placement.Metadata.ProvisioningStatusDetail = &reservationapi.ProvisioningStatusDetail{
		Message: "rebuild of server slot 0 was not accepted",
	}

	return placement
}

func TestWaitForPlacementUpdateRidesOutTransientFailure(t *testing.T) {
	setPlacementFailureGrace(t, time.Hour)

	observed := updateTestPlacement()

	got, diagnostics := waitForPlacementUpdate(
		t.Context(),
		10*time.Second,
		unobserved(),
		reads(erroredBeforeObserved(), erroredBeforeObserved(), observed),
	)
	if diagnostics.HasError() {
		t.Fatalf("waitForPlacementUpdate() diagnostics = %v", diagnostics)
	}

	if got != observed {
		t.Errorf("waitForPlacementUpdate() = %p, want the observed read %p", got, observed)
	}
}

func serverErrorThen(
	n int,
	placement *reservationapi.PlacementV2Read,
) func(context.Context) (*reservationapi.PlacementV2Read, error) {
	calls := 0

	return func(context.Context) (*reservationapi.PlacementV2Read, error) {
		calls++
		if calls <= n {
			return nil, &nscale.APIError{StatusCode: http.StatusServiceUnavailable}
		}

		return placement, nil
	}
}

func TestWaitForPlacementUpdateRidesOutServerError(t *testing.T) {
	setPlacementFailureGrace(t, time.Hour)

	observed := updateTestPlacement()

	got, diagnostics := waitForPlacementUpdate(t.Context(), 10*time.Second, unobserved(), serverErrorThen(2, observed))
	if diagnostics.HasError() {
		t.Fatalf("waitForPlacementUpdate() diagnostics = %v", diagnostics)
	}

	if got != observed {
		t.Errorf("waitForPlacementUpdate() = %p, want the observed read %p", got, observed)
	}
}

func TestWaitForPlacementUpdateKeepsPendingThroughServerErrors(t *testing.T) {
	setPlacementFailureGrace(t, time.Hour)

	_, diagnostics := waitForPlacementUpdate(t.Context(), time.Second, unobserved(), serverErrorThen(1000, nil))
	if !diagnostics.HasError() {
		t.Fatal("waitForPlacementUpdate() diagnostics = none, want a timeout")
	}

	if detail := diagnostics.Errors()[0].Detail(); !strings.Contains(
		detail,
		"last state: '"+placementUpdatePending+"'",
	) {
		t.Errorf("diagnostic detail = %q, want a timeout while %s", detail, placementUpdatePending)
	}
}

func TestWaitForPlacementUpdateFailsPersistentServerError(t *testing.T) {
	setPlacementFailureGrace(t, 0)

	_, diagnostics := waitForPlacementUpdate(
		t.Context(),
		10*time.Second,
		unobserved(),
		serverErrorThen(1, updateTestPlacement()),
	)
	if !diagnostics.HasError() {
		t.Fatal("waitForPlacementUpdate() diagnostics = none, want an error")
	}
}

func TestWaitForPlacementUpdateFailsPersistentFailure(t *testing.T) {
	setPlacementFailureGrace(t, 0)

	_, diagnostics := waitForPlacementUpdate(t.Context(), 10*time.Second, unobserved(), reads(erroredBeforeObserved()))
	if !diagnostics.HasError() {
		t.Fatal("waitForPlacementUpdate() diagnostics = none, want an error")
	}

	if detail := diagnostics.Errors()[0].Detail(); !strings.Contains(
		detail,
		"rebuild of server slot 0 was not accepted",
	) {
		t.Errorf("diagnostic detail = %q, want the API's provisioning status message", detail)
	}
}

type fakePlacementAPI struct {
	mu        sync.Mutex
	placement *reservationapi.PlacementV2Read
	updates   []json.RawMessage
	reads     int

	conflictsLeft int

	reject bool
}

func (f *fakePlacementAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.URL.Path != "/api/v2/placements/"+f.placement.Metadata.Id {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		f.reads++
		f.placement.Status.StatusCurrent = true
	case http.MethodPut:
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		f.updates = append(f.updates, body)

		if f.conflictsLeft > 0 {
			f.conflictsLeft--

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"conflict","error_description":"placement was modified concurrently"}`))

			return
		}

		if f.reject {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_request","error_description":"only the image may change"}`))

			return
		}

		var update reservationapi.PlacementV2Update
		if err := json.Unmarshal(body, &update); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		f.placement.Spec = update.Spec
		f.placement.Status.StatusCurrent = false
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(f.placement)
}

func updateThroughFakeAPI(
	t *testing.T,
	api *fakePlacementAPI,
	imageID string,
) (*reservationapi.PlacementV2Read, diag.Diagnostics) {
	t.Helper()

	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	planned := updateTestPlacement()
	planned.Spec.ServerSpec.ImageId = imageID
	plan := PlacementResourceModel{PlacementModel: NewPlacementModel(planned)}

	return updateThroughFakeAPIFrom(t, server.URL, plan)
}

func updateThroughFakeAPIFrom(
	t *testing.T,
	url string,
	plan PlacementResourceModel,
) (*reservationapi.PlacementV2Read, diag.Diagnostics) {
	t.Helper()

	reservationClient, err := reservationapi.NewClient(url)
	if err != nil {
		t.Fatalf("reservation client: %v", err)
	}

	return placementUpdateAndWait(
		t.Context(),
		&nscale.Client{Reservation: reservationClient},
		"placement-1",
		plan,
		10*time.Second,
	)
}

func TestPlacementUpdateAndWait(t *testing.T) {
	api := &fakePlacementAPI{placement: updateTestPlacement()}

	got, diagnostics := updateThroughFakeAPI(t, api, updateTestNewImageID)
	if diagnostics.HasError() {
		t.Fatalf("placementUpdateAndWait() diagnostics = %v", diagnostics)
	}

	if got.Spec.ServerSpec.ImageId != updateTestNewImageID || !got.Status.StatusCurrent {
		t.Errorf("final read image = %q, statusCurrent = %t; want %q, true",
			got.Spec.ServerSpec.ImageId, got.Status.StatusCurrent, updateTestNewImageID)
	}

	if len(api.updates) != 1 {
		t.Fatalf("updates sent = %d, want 1", len(api.updates))
	}

	var sent struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(api.updates[0], &sent); err != nil {
		t.Fatalf("decode update: %v", err)
	}

	want := updateTestPlacement().Spec
	want.ServerSpec.ImageId = updateTestNewImageID

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("encode want: %v", err)
	}

	if string(sent.Spec) != string(wantJSON) {
		t.Errorf("update spec = %s, want %s", sent.Spec, wantJSON)
	}
}

func TestPlacementUpdateAndWaitRetriesConflict(t *testing.T) {
	api := &fakePlacementAPI{placement: updateTestPlacement(), conflictsLeft: 1}

	got, diagnostics := updateThroughFakeAPI(t, api, updateTestNewImageID)
	if diagnostics.HasError() {
		t.Fatalf("placementUpdateAndWait() diagnostics = %v", diagnostics)
	}

	if got.Spec.ServerSpec.ImageId != updateTestNewImageID {
		t.Errorf("final read image = %q, want %q", got.Spec.ServerSpec.ImageId, updateTestNewImageID)
	}

	if len(api.updates) != 2 {
		t.Errorf("updates sent = %d, want 2", len(api.updates))
	}

	if wantReads := len(api.updates) + 1; api.reads < wantReads {
		t.Errorf("reads = %d, want at least %d: one before each update, then the wait", api.reads, wantReads)
	}
}

func TestPlacementUpdateAndWaitGivesUpOnRepeatedConflicts(t *testing.T) {
	api := &fakePlacementAPI{placement: updateTestPlacement(), conflictsLeft: placementUpdateAttempts}

	_, diagnostics := updateThroughFakeAPI(t, api, updateTestNewImageID)
	if !diagnostics.HasError() {
		t.Fatal("placementUpdateAndWait() diagnostics = none, want an error")
	}

	if len(api.updates) != placementUpdateAttempts {
		t.Errorf("updates sent = %d, want %d", len(api.updates), placementUpdateAttempts)
	}

	if detail := diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "modified concurrently") {
		t.Errorf("diagnostic detail = %q, want the API's conflict message", detail)
	}
}

func TestPlacementUpdateAndWaitDoesNotRetryRejection(t *testing.T) {
	api := &fakePlacementAPI{placement: updateTestPlacement(), reject: true}

	_, diagnostics := updateThroughFakeAPI(t, api, updateTestNewImageID)
	if !diagnostics.HasError() {
		t.Fatal("placementUpdateAndWait() diagnostics = none, want an error")
	}

	if len(api.updates) != 1 {
		t.Errorf("updates sent = %d, want 1", len(api.updates))
	}

	if detail := diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "only the image may change") {
		t.Errorf("diagnostic detail = %q, want the API's rejection message", detail)
	}
}

func TestPlacementUpdateAndWaitSkipsUnchangedSpec(t *testing.T) {
	for _, imageID := range []string{updateTestImageID, strings.ToUpper(updateTestImageID)} {
		t.Run(imageID, func(t *testing.T) {
			api := &fakePlacementAPI{placement: updateTestPlacement()}

			got, diagnostics := updateThroughFakeAPI(t, api, imageID)
			if diagnostics.HasError() {
				t.Fatalf("placementUpdateAndWait() diagnostics = %v", diagnostics)
			}

			if len(api.updates) != 0 {
				t.Errorf("updates sent = %d, want none", len(api.updates))
			}

			if api.reads != 1 {
				t.Errorf("reads = %d, want only the update's own read", api.reads)
			}

			if got.Spec.ServerSpec.ImageId != updateTestImageID {
				t.Errorf("returned image = %q, want %q", got.Spec.ServerSpec.ImageId, updateTestImageID)
			}
		})
	}
}

func TestPlacementUpdateAndWaitSendsChangedStrategy(t *testing.T) {
	api := &fakePlacementAPI{placement: updateTestPlacement()}

	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	planned := updateTestPlacement()
	planned.Spec.UpdateStrategy = &reservationapi.PlacementUpdateStrategyV2{
		Type:          reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate,
		RollingUpdate: &reservationapi.PlacementRollingUpdateV2{MaxUnavailable: new("1")},
	}
	plan := PlacementResourceModel{PlacementModel: NewPlacementModel(planned)}

	got, diagnostics := updateThroughFakeAPIFrom(t, server.URL, plan)
	if diagnostics.HasError() {
		t.Fatalf("placementUpdateAndWait() diagnostics = %v", diagnostics)
	}

	if got.Spec.UpdateStrategy.Type != reservationapi.PlacementUpdateStrategyTypeV2RollingUpdate {
		t.Errorf("final strategy = %s, want RollingUpdate", got.Spec.UpdateStrategy.Type)
	}
}
