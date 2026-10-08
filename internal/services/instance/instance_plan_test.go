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

package instance_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	computeapi "github.com/nscaledev/nscale-sdk-go/compute"

	"github.com/nscaledev/terraform-provider-nscale/internal/provider"
	"github.com/nscaledev/terraform-provider-nscale/internal/services/instance"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/uuidtype"
)

// The API stores the instance's identifiers canonicalised, so respelling them
// in configuration is no change: neither an update nor, for the SSH
// certificate authority, a replacement.
func TestInstanceRespelledUUIDsPlanNoChange(t *testing.T) {
	ctx := context.Background()

	imageID := uuid.MustParse("5c9b2e7f-1a4d-4b8c-8f2e-3d6a9b0c1e52")
	flavorID := uuid.MustParse("2f8a1d4e-6b3c-4f5a-9e0d-7c1b8a2f3d40")
	caID := uuid.MustParse("8e3f4a1b-2c5d-4e7f-9a0b-1c2d3e4f5a60")

	prior := instance.InstanceResourceModel{
		InstanceModel: instance.NewInstanceModel(&computeapi.InstanceRead{
			Metadata: computeapi.ProjectScopedResourceReadMetadata{
				Id:           "instance-1",
				Name:         "test-instance",
				ProjectId:    "project-1",
				CreationTime: time.Date(2026, time.April, 28, 11, 3, 12, 0, time.UTC),
			},
			Spec: computeapi.InstanceSpec{
				FlavorId:                  flavorID,
				ImageId:                   imageID,
				SshCertificateAuthorityId: &caID,
				Networking:                &computeapi.InstanceNetworking{},
			},
			Status: computeapi.InstanceStatus{
				NetworkId: "network-1",
				RegionId:  "region-1",
			},
		}),
		Timeouts: tftimeouts.Value{
			Object: types.ObjectNull(map[string]attr.Type{
				"create": types.StringType,
				"update": types.StringType,
				"delete": types.StringType,
			}),
		},
	}

	proposed := prior
	proposed.ImageID = uuidtype.NewValue(strings.ToUpper(imageID.String()))
	proposed.FlavorID = uuidtype.NewValue(strings.ToUpper(flavorID.String()))
	proposed.SSHCertificateAuthorityID = uuidtype.NewValue(strings.ToUpper(caID.String()))

	config := proposed
	config.ID = types.StringNull()
	config.PublicIP = types.StringNull()
	config.PrivateIP = types.StringNull()
	config.PowerState = types.StringNull()
	config.RegionID = types.StringNull()
	config.CreationTime = types.StringNull()

	var schemaResponse resource.SchemaResponse
	instance.NewInstanceResource().Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
	schemaType := schemaResponse.Schema.Type().TerraformType(ctx)

	encode := func(model instance.InstanceResourceModel) tftypes.Value {
		t.Helper()

		plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaType, nil)}
		if diagnostics := plan.Set(ctx, model); diagnostics.HasError() {
			t.Fatalf("encode model: %v", diagnostics)
		}

		return plan.Raw
	}

	dynamic := func(model instance.InstanceResourceModel) *tfprotov6.DynamicValue {
		t.Helper()

		value, err := tfprotov6.NewDynamicValue(schemaType, encode(model))
		if err != nil {
			t.Fatalf("encode model: %v", err)
		}

		return &value
	}

	server, err := providerserver.NewProtocol6WithError(provider.New())()
	if err != nil {
		t.Fatalf("provider server: %v", err)
	}

	response, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "nscale_instance",
		PriorState:       dynamic(prior),
		ProposedNewState: dynamic(proposed),
		Config:           dynamic(config),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange() error = %v", err)
	}

	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("PlanResourceChange() diagnostic: %s: %s", diagnostic.Summary, diagnostic.Detail)
		}
	}

	if len(response.RequiresReplace) != 0 {
		t.Errorf("RequiresReplace = %v, want none", response.RequiresReplace)
	}

	planned, err := response.PlannedState.Unmarshal(schemaType)
	if err != nil {
		t.Fatalf("decode planned state: %v", err)
	}

	if !planned.Equal(encode(prior)) {
		t.Error("plan changes the instance, want no change")
	}
}
