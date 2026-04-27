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

package securitygroup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	computeapi "github.com/unikorn-cloud/compute/pkg/openapi"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
)

func TestFindInstancesUsingSecurityGroup(t *testing.T) {
	const (
		targetSGID    = "sg-target"
		otherSGID     = "sg-other"
		matchingID    = "instance-match"
		nonMatchingID = "instance-no-match"
		networkID     = "net-1"
		orgID         = "org-1"
		projectID     = "project-1"
		regionID      = "region-1"
	)

	makeInstance := func(id string, sgs []string) computeapi.InstanceRead {
		var securityGroupsPtr *computeapi.SecurityGroupIDList
		if sgs != nil {
			list := computeapi.SecurityGroupIDList(sgs)
			securityGroupsPtr = &list
		}
		return computeapi.InstanceRead{
			Metadata: coreapi.ProjectScopedResourceReadMetadata{Id: id},
			Spec: computeapi.InstanceSpec{
				Networking: &computeapi.InstanceNetworking{
					SecurityGroups: securityGroupsPtr,
				},
			},
		}
	}

	cases := []struct {
		name      string
		instances computeapi.InstancesRead
		want      []string
	}{
		{
			name: "match",
			instances: computeapi.InstancesRead{
				makeInstance(matchingID, []string{otherSGID, targetSGID}),
				makeInstance(nonMatchingID, []string{otherSGID}),
			},
			want: []string{matchingID},
		},
		{
			name: "no match",
			instances: computeapi.InstancesRead{
				makeInstance(nonMatchingID, []string{otherSGID}),
			},
			want: nil,
		},
		{
			name: "nil networking",
			instances: computeapi.InstancesRead{
				{
					Metadata: coreapi.ProjectScopedResourceReadMetadata{Id: nonMatchingID},
					Spec:     computeapi.InstanceSpec{Networking: nil},
				},
			},
			want: nil,
		},
		{
			name: "nil security groups",
			instances: computeapi.InstancesRead{
				makeInstance(nonMatchingID, nil),
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/instances" {
					t.Errorf("unexpected request path: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				query := r.URL.Query()
				if got := query["organizationID"]; !slices.Equal(got, []string{orgID}) {
					t.Errorf("organizationID query = %v, want [%s]", got, orgID)
				}
				if got := query["projectID"]; !slices.Equal(got, []string{projectID}) {
					t.Errorf("projectID query = %v, want [%s]", got, projectID)
				}
				if got := query["regionID"]; !slices.Equal(got, []string{regionID}) {
					t.Errorf("regionID query = %v, want [%s]", got, regionID)
				}
				if got := query["networkID"]; !slices.Equal(got, []string{networkID}) {
					t.Errorf("networkID query = %v, want [%s]", got, networkID)
				}

				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(tc.instances); err != nil {
					t.Fatalf("encode response: %s", err)
				}
			}))
			t.Cleanup(server.Close)

			compute, err := computeapi.NewClient(server.URL)
			if err != nil {
				t.Fatalf("NewClient: %s", err)
			}

			client := &nscale.Client{
				OrganizationID: orgID,
				ProjectID:      projectID,
				RegionID:       regionID,
				Compute:        compute,
			}

			got, err := findInstancesUsingSecurityGroup(context.Background(), client, networkID, targetSGID)
			if err != nil {
				t.Fatalf("findInstancesUsingSecurityGroup: %s", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
