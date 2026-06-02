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

package identity

import (
	"context"

	identityapi "github.com/nscaledev/nscale-sdk-go/identity"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

func getGroup(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*identityapi.GroupRead, error) {
	groupResponse, err := client.Identity.GetApiV1OrganizationsOrganizationIDGroupsGroupid(
		ctx,
		client.OrganizationID,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer groupResponse.Body.Close()

	group, err := nscale.ReadJSONResponsePointer[identityapi.GroupRead](groupResponse)
	if err != nil {
		return nil, err
	}

	return group, nil
}

// getGroupStatus reads a group and adapts it to the shared watchers'
// (resource, ResourceStatus, error) shape. Identity reads are
// organization-scoped, so it uses StatusFromOrgScoped.
func getGroupStatus(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*identityapi.GroupRead, nscale.ResourceStatus, error) {
	group, err := getGroup(ctx, id, client)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	return group, nscale.StatusFromOrgScoped(&group.Metadata), nil
}
