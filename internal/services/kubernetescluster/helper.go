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

package kubernetescluster

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"

	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// Tags go through the shared generic helpers in internal/utils/tftypes and
// internal/nscale rather than NKS-local copies. tags.SDKTag is a structural
// constraint, so kubernetesapi.Tag satisfies it on its shape alone and needs no
// per-service conversion of its own — the same route every other service takes.

// basetypesObjectOptions is the conversion policy for unpacking the nested
// config objects. Both flags stay false deliberately: a null or unknown nested
// object must stay distinguishable from an empty one, because "omitted" is what
// tells the converters to send nothing and let the API apply its own defaults.
// Coercing either to an empty struct would send explicit zero values instead
// and silently override those defaults.
func basetypesObjectOptions() basetypes.ObjectAsOptions {
	return basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    false,
		UnhandledUnknownAsEmpty: false,
	}
}

// stringSliceValue maps an API string slice onto a Terraform list, preserving
// the API's ordering. An empty-but-present slice becomes an empty list rather
// than null: for eligibleTargets the API documents empty as "eligibility was
// observed and there is no upgrade", which is a different fact from "we never
// observed eligibility".
func stringSliceValue(values []string) basetypes.ListValue {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}

	return basetypes.NewListValueMust(types.StringType, elements)
}

// stringSliceValuePointer is the nullable counterpart of stringSliceValue, for
// optional API slices where absent and empty carry different meanings.
func stringSliceValuePointer(values *[]string) basetypes.ListValue {
	if values == nil {
		return basetypes.NewListNull(types.StringType)
	}

	return stringSliceValue(*values)
}

// getCluster reads one cluster by ID. It returns the decoded body plus the
// metadata and status the waiters need, so callers get the freshness inputs
// without a second request.
func getCluster(
	ctx context.Context,
	client *nscale.Client,
	id string,
) (*kubernetesapi.ClusterV1Read, error) {
	nksClient, diagnostics := client.RequireNKS()
	if diagnostics.HasError() {
		return nil, nscale.DiagnosticsError(diagnostics)
	}

	response, err := nksClient.GetCluster(ctx, id)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	return nscale.ReadJSONResponsePointer[kubernetesapi.ClusterV1Read](response)
}
