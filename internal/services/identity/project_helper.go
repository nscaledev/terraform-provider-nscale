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
	identityids "github.com/unikorn-cloud/identity/pkg/ids"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

func getProject(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*identityapi.ProjectRead, error) {
	organizationID, err := identityids.ParseOrganizationID(client.OrganizationID)
	if err != nil {
		return nil, err
	}

	projectID, err := identityids.ParseProjectID(id)
	if err != nil {
		return nil, err
	}

	projectResponse, err := client.Identity.GetApiV1OrganizationsOrganizationIDProjectsProjectID(
		ctx,
		organizationID,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer projectResponse.Body.Close()

	project, err := nscale.ReadJSONResponsePointer[identityapi.ProjectRead](projectResponse)
	if err != nil {
		return nil, err
	}

	return project, nil
}

// getProjectStatus reads a project and adapts it to the shared watchers'
// (resource, ResourceStatus, error) shape. Identity reads are
// organization-scoped, so it uses StatusFromOrgScoped.
func getProjectStatus(
	ctx context.Context,
	id string,
	client *nscale.Client,
) (*identityapi.ProjectRead, nscale.ResourceStatus, error) {
	project, err := getProject(ctx, id, client)
	if err != nil {
		return nil, nscale.ResourceStatus{}, err
	}

	return project, nscale.StatusFromOrgScoped(&project.Metadata), nil
}
