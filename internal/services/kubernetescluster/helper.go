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
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/nscaledev/terraform-provider-nscale/internal/nks"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
)

// The tag helpers below duplicate internal/utils/tftypes' TagMapValueMust and
// ValueTagListPointer. They exist because those are typed on
// nscale-sdk-go/common.Tag while NKS generates its own structurally-identical
// nks.Tag, and Go has no structural typing to bridge them. Rather than make the
// shared helpers generic for one caller, keep the NKS-local copies here and
// delete them when the SDK-wide v0.2.0 migration collapses the two Tag types
// into one — see internal/nks/gen.go.

func tagMapValueMust(tags *nks.TagList) basetypes.MapValue {
	if tags == nil || len(*tags) == 0 {
		return basetypes.NewMapNull(types.StringType)
	}

	elements := make(map[string]attr.Value, len(*tags))
	for _, tag := range *tags {
		elements[tag.Name] = types.StringValue(tag.Value)
	}

	return basetypes.NewMapValueMust(types.StringType, elements)
}

func valueTagListPointer(ctx context.Context, tagMap basetypes.MapValue) (*nks.TagList, diag.Diagnostics) {
	if tagMap.IsNull() || tagMap.IsUnknown() {
		return nil, nil
	}

	var data map[string]string
	if diagnostics := tagMap.ElementsAs(ctx, &data, false); diagnostics.HasError() {
		return nil, diagnostics
	}

	if len(data) == 0 {
		return nil, nil
	}

	tags := make(nks.TagList, 0, len(data))
	for name, value := range data {
		tags = append(tags, nks.Tag{Name: name, Value: value})
	}

	return &tags, nil
}

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

// isSettled reports whether a cluster's status has caught up with its spec.
//
// NKS writes are asynchronous AND its status projection lags independently: for
// a window after any write, metadata.provisioningStatus still describes the
// PREVIOUS generation of the spec. Trusting it during that window is how a
// create or update returns "success" while the computed attributes it wrote to
// state (endpoints, kubernetes version, applied release) still describe the old
// cluster. observedGeneration is the API's own freshness marker, and comparing
// it against metadata.generation is the only reliable way to know the status
// being read corresponds to the spec we just sent.
//
// A nil observedGeneration means no projection has completed yet — treat as
// not settled, never as settled-at-zero.
//
// This is the NKS-native equivalent of the operation-tag round-trip that
// internal/nscale's UpdateStateWatcher uses for the cache-backed region API.
// observedGeneration is strictly better: nothing is written into user-visible
// tags, so there is no strip-on-read step, and it covers create as well as
// update.
func isSettled(metadata *nks.ProjectScopedResourceReadMetadataV1, status *nks.ClusterStatusV1) bool {
	if metadata == nil || status == nil || status.ObservedGeneration == nil {
		return false
	}

	return *status.ObservedGeneration >= metadata.Generation
}

// getCluster reads one cluster by ID. It returns the decoded body plus the
// metadata and status the waiters need, so callers get the freshness inputs
// without a second request.
func getCluster(
	ctx context.Context,
	client *nscale.Client,
	id string,
) (*nks.ClusterV1Read, error) {
	nksClient, diagnostics := client.RequireNKS()
	if diagnostics.HasError() {
		return nil, diagnosticsError(diagnostics)
	}

	response, err := nksClient.GetCluster(ctx, id)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	return nscale.ReadJSONResponsePointer[nks.ClusterV1Read](response)
}

// diagnosticsError flattens diagnostics into an error, for the paths that must
// return the (value, error) shape the shared readers and watchers expect. Only
// used for the RequireNKS guard, which is a configuration error rather than an
// API failure.
func diagnosticsError(diagnostics diag.Diagnostics) error {
	if !diagnostics.HasError() {
		return nil
	}

	first := diagnostics.Errors()[0]

	return fmt.Errorf("%s: %s", first.Summary(), first.Detail())
}

// isNotFound reports whether err is an API 404. Delete tolerates it (the
// cluster is already gone) and Read treats it as "remove from state".
func isNotFound(err error) bool {
	e, ok := nscale.AsAPIError(err)

	return ok && e.StatusCode == http.StatusNotFound
}
