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

package kubernetesnodepool

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	kubernetesapi "github.com/nscaledev/nscale-sdk-go/kubernetes"
)

// provisioning_mode does double duty: it gates which capacity block is valid,
// and it gates whether replicas, taints and labels force replacement. Both
// readings live in this file so the two can never disagree about what mode a
// pool is in.

// Attribute names for the two capacity blocks, used for both paths and error
// messages.
const (
	computeAttribute     = "compute"
	reservationAttribute = "reservation"
)

// configuredMode reads provisioning_mode out of a config. An unknown or absent
// value yields the empty string, which every caller below treats as "cannot
// decide yet" rather than as a mode.
func configuredMode(ctx context.Context, config tfsdk.Config) (string, diag.Diagnostics) {
	var mode types.String

	diagnostics := config.GetAttribute(ctx, path.Root("provisioning_mode"), &mode)
	if diagnostics.HasError() || mode.IsNull() || mode.IsUnknown() {
		return "", diagnostics
	}

	return mode.ValueString(), diagnostics
}

// isReservationMode reports whether the planned pool is reservation-backed.
func isReservationMode(ctx context.Context, config tfsdk.Config) (bool, diag.Diagnostics) {
	mode, diagnostics := configuredMode(ctx, config)

	return mode == string(kubernetesapi.NodePoolProvisioningModeV1Reservation), diagnostics
}

// A reservation pool is backed by a placement, and a placement never rolls:
// CAPNS documents no in-place resize, no rolling update and no per-node drain
// for that backend. Nothing a template change would express can take effect, so
// the API rejects the edit rather than accepting a change that could never
// land. replicas, taints and labels are therefore immutable in reservation mode
// and in-place in compute mode, which is a RequiresReplaceIf rather than a flat
// rule.
//
// Reading the mode from config rather than state is safe: provisioning_mode is
// itself RequiresReplace, so a mode change replaces the pool regardless of
// which side this predicate looks at.
const replaceIfReservationDescription = "Changing this forces a new node pool when `provisioning_mode` is " +
	"`reservation`, because a reservation-backed pool cannot be modified in place."

// replicasRequiresReplaceIfReservation is the replicas plan modifier. Scaling a
// reservation pool is a rebuild, not a scale: it releases and re-claims the
// placement.
func replicasRequiresReplaceIfReservation() planmodifier.Int64 {
	return int64planmodifier.RequiresReplaceIf(
		func(
			ctx context.Context,
			request planmodifier.Int64Request,
			response *int64planmodifier.RequiresReplaceIfFuncResponse,
		) {
			reservation, diagnostics := isReservationMode(ctx, request.Config)
			response.Diagnostics.Append(diagnostics...)
			response.RequiresReplace = reservation
		},
		replaceIfReservationDescription,
		replaceIfReservationDescription,
	)
}

func taintsRequiresReplaceIfReservation() planmodifier.List {
	return listplanmodifier.RequiresReplaceIf(
		func(
			ctx context.Context,
			request planmodifier.ListRequest,
			response *listplanmodifier.RequiresReplaceIfFuncResponse,
		) {
			reservation, diagnostics := isReservationMode(ctx, request.Config)
			response.Diagnostics.Append(diagnostics...)
			response.RequiresReplace = reservation
		},
		replaceIfReservationDescription,
		replaceIfReservationDescription,
	)
}

func labelsRequiresReplaceIfReservation() planmodifier.Map {
	return mapplanmodifier.RequiresReplaceIf(
		func(
			ctx context.Context,
			request planmodifier.MapRequest,
			response *mapplanmodifier.RequiresReplaceIfFuncResponse,
		) {
			reservation, diagnostics := isReservationMode(ctx, request.Config)
			response.Diagnostics.Append(diagnostics...)
			response.RequiresReplace = reservation
		},
		replaceIfReservationDescription,
		replaceIfReservationDescription,
	)
}

// capacityModeIssue names what is wrong with a mode and capacity-block
// combination.
type capacityModeIssue int

const (
	// capacityModeMissing is the block the mode requires being absent.
	capacityModeMissing capacityModeIssue = iota
	// capacityModeUnexpected is the block belonging to the other mode being
	// present.
	capacityModeUnexpected
)

// capacityModeFinding is one problem with the configured combination.
type capacityModeFinding struct {
	Attribute string
	Issue     capacityModeIssue
}

// checkCapacityMode is the whole mode-versus-blocks decision.
//
// Kept as a pure function over three booleans so the four-row matrix can be
// unit tested without standing up a tfsdk.Config — the plumbing that reads
// those booleans is in validateCapacityMode below.
//
// An empty mode returns no findings: an unresolved mode cannot be validated
// against, and stringvalidator.OneOf on the attribute itself already rejects a
// bad literal.
func checkCapacityMode(mode string, computeSet, reservationSet bool) []capacityModeFinding {
	var required, forbidden string
	var requiredSet, forbiddenSet bool

	switch mode {
	case string(kubernetesapi.NodePoolProvisioningModeV1Compute):
		required, requiredSet = computeAttribute, computeSet
		forbidden, forbiddenSet = reservationAttribute, reservationSet
	case string(kubernetesapi.NodePoolProvisioningModeV1Reservation):
		required, requiredSet = reservationAttribute, reservationSet
		forbidden, forbiddenSet = computeAttribute, computeSet
	default:
		return nil
	}

	var findings []capacityModeFinding

	if !requiredSet {
		findings = append(findings, capacityModeFinding{Attribute: required, Issue: capacityModeMissing})
	}
	if forbiddenSet {
		findings = append(findings, capacityModeFinding{Attribute: forbidden, Issue: capacityModeUnexpected})
	}

	return findings
}

// validateCapacityMode enforces that exactly the capacity block matching
// provisioning_mode is set.
//
// This runs at plan time on purpose. The API rejects a mismatch, and the ticket
// is explicit that a mis-configuration must fail at plan or validate rather
// than at apply — a user who has already waited for a cluster should not
// discover a typo by watching a pool create fail.
//
// The framework's resourcevalidator.Conflicting would catch "both blocks set"
// but not "the wrong one set", and neither it nor ExactlyOneOf knows which
// block a given mode requires. Hence the hand-written decision above.
func validateCapacityMode(
	ctx context.Context,
	config tfsdk.Config,
	diagnostics *diag.Diagnostics,
) {
	mode, modeDiagnostics := configuredMode(ctx, config)
	if diagnostics.Append(modeDiagnostics...); diagnostics.HasError() {
		return
	}

	computeSet := blockIsSet(ctx, config, computeAttribute, diagnostics)
	reservationSet := blockIsSet(ctx, config, reservationAttribute, diagnostics)
	if diagnostics.HasError() {
		return
	}

	for _, finding := range checkCapacityMode(mode, computeSet, reservationSet) {
		attribute := path.Root(finding.Attribute)

		switch finding.Issue {
		case capacityModeMissing:
			diagnostics.AddAttributeError(
				attribute,
				"Missing Node Pool Capacity Block",
				fmt.Sprintf(
					"`%s` must be set when `provisioning_mode` is %q, because it is where the pool's "+
						"worker capacity comes from.",
					finding.Attribute, mode,
				),
			)
		case capacityModeUnexpected:
			diagnostics.AddAttributeError(
				attribute,
				"Unexpected Node Pool Capacity Block",
				fmt.Sprintf(
					"`%s` must be omitted when `provisioning_mode` is %q. A node pool takes its capacity "+
						"from exactly one source, and the API rejects a spec carrying the other one.",
					finding.Attribute, mode,
				),
			)
		}
	}
}

// blockIsSet reports whether a capacity block is present in the configuration.
//
// An unknown block counts as set: it is written in the configuration, which is
// all this check is about, even though its value is not resolved yet.
func blockIsSet(
	ctx context.Context,
	config tfsdk.Config,
	attribute string,
	diagnostics *diag.Diagnostics,
) bool {
	var block types.Object

	blockDiagnostics := config.GetAttribute(ctx, path.Root(attribute), &block)
	if diagnostics.Append(blockDiagnostics...); diagnostics.HasError() {
		return false
	}

	return !block.IsNull()
}
