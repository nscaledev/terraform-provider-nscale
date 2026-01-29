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

package network

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/nscaledev/terraform-provider-nscale/internal/nscale"
	"github.com/nscaledev/terraform-provider-nscale/internal/utils/tftypes"
	coreapi "github.com/unikorn-cloud/core/pkg/openapi"
	regionapi "github.com/unikorn-cloud/region/pkg/openapi"
)

type NetworkModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	DNSNameservers types.List   `tfsdk:"dns_nameservers"`
	Routes         types.List   `tfsdk:"routes"`
	CIDRBlock      types.String `tfsdk:"cidr_block"`
	Tags           types.Map    `tfsdk:"tags"`
	RegionID       types.String `tfsdk:"region_id"`
	CreationTime   types.String `tfsdk:"creation_time"`
}

func NewNetworkModel(source *regionapi.NetworkV2Read) NetworkModel {
	dnsNameservers := make([]attr.Value, 0, len(source.Spec.DnsNameservers))
	for _, dnsNameserver := range source.Spec.DnsNameservers {
		dnsNameservers = append(dnsNameservers, types.StringValue(dnsNameserver))
	}

	routes := types.ListNull(RouteModelAttributeType)
	if source.Spec.Routes != nil {
		routes = NewRouteModels(*source.Spec.Routes)
	}

	tags := nscale.RemoveOperationTags(source.Metadata.Tags)

	return NetworkModel{
		ID:             types.StringValue(source.Metadata.Id),
		Name:           types.StringValue(source.Metadata.Name),
		Description:    types.StringPointerValue(source.Metadata.Description),
		DNSNameservers: tftypes.NullableListValueMust(types.StringType, dnsNameservers),
		Routes:         routes,
		CIDRBlock:      types.StringValue(source.Status.Prefix),
		Tags:           tftypes.TagMapValueMust(tags),
		RegionID:       types.StringValue(source.Status.RegionId),
		CreationTime:   types.StringValue(source.Metadata.CreationTime.Format(time.RFC3339)),
	}
}

func (m *NetworkModel) NscaleNetworkCreateParams(organizationID, projectID string) (regionapi.NetworkV2Create, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return regionapi.NetworkV2Create{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	var dnsNameservers []string
	if diagnostics = m.DNSNameservers.ElementsAs(context.TODO(), &dnsNameservers, false); diagnostics.HasError() {
		return regionapi.NetworkV2Create{}, diagnostics
	}

	var sourceRoutes []RouteModel
	if diagnostics = m.Routes.ElementsAs(context.TODO(), &sourceRoutes, false); diagnostics.HasError() {
		return regionapi.NetworkV2Create{}, diagnostics
	}

	routes := make([]regionapi.Route, 0, len(sourceRoutes))
	for _, source := range sourceRoutes {
		routes = append(routes, source.NscaleRoute())
	}

	var nonEmptyRoutes *[]regionapi.Route
	if len(routes) > 0 {
		nonEmptyRoutes = &routes
	}

	network := regionapi.NetworkV2Create{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: regionapi.NetworkV2CreateSpec{
			DnsNameservers: dnsNameservers,
			OrganizationId: organizationID,
			Prefix:         m.CIDRBlock.ValueString(),
			ProjectId:      projectID,
			RegionId:       m.RegionID.ValueString(),
			Routes:         nonEmptyRoutes,
		},
	}

	return network, nil
}

func (m *NetworkModel) NscaleNetworkUpdateParams() (regionapi.NetworkV2Update, diag.Diagnostics) {
	tags, diagnostics := tftypes.ValueTagListPointer(m.Tags)
	if diagnostics.HasError() {
		return regionapi.NetworkV2Update{}, diagnostics
	}

	tags = nscale.RemoveOperationTags(tags)

	var dnsNameservers []string
	if diagnostics = m.DNSNameservers.ElementsAs(context.TODO(), &dnsNameservers, false); diagnostics.HasError() {
		return regionapi.NetworkV2Update{}, diagnostics
	}

	var sourceRoutes []RouteModel
	if diagnostics = m.Routes.ElementsAs(context.TODO(), &sourceRoutes, false); diagnostics.HasError() {
		return regionapi.NetworkV2Update{}, diagnostics
	}

	routes := make([]regionapi.Route, 0, len(sourceRoutes))
	for _, source := range sourceRoutes {
		routes = append(routes, source.NscaleRoute())
	}

	var nonEmptyRoutes *[]regionapi.Route
	if len(routes) > 0 {
		nonEmptyRoutes = &routes
	}

	network := regionapi.NetworkV2Update{
		Metadata: coreapi.ResourceWriteMetadata{
			Description: m.Description.ValueStringPointer(),
			Name:        m.Name.ValueString(),
			Tags:        tags,
		},
		Spec: regionapi.NetworkV2Spec{
			DnsNameservers: dnsNameservers,
			Routes:         nonEmptyRoutes,
		},
	}

	return network, nil
}

var RouteModelAttributeType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"destination": types.StringType,
		"nexthop":     types.StringType,
	},
}

type RouteModel struct {
	Destination types.String `tfsdk:"destination"`
	NextHop     types.String `tfsdk:"nexthop"`
}

func NewRouteModel(source regionapi.Route) attr.Value {
	return types.ObjectValueMust(
		RouteModelAttributeType.AttrTypes,
		map[string]attr.Value{
			"destination": types.StringValue(source.Prefix),
			"nexthop":     types.StringValue(source.Nexthop),
		},
	)
}

func NewRouteModels(source []regionapi.Route) types.List {
	routes := make([]attr.Value, 0, len(source))
	for _, data := range source {
		routes = append(routes, NewRouteModel(data))
	}
	return types.ListValueMust(RouteModelAttributeType, routes)
}

func (m *RouteModel) NscaleRoute() regionapi.Route {
	return regionapi.Route{
		Nexthop: m.NextHop.ValueString(),
		Prefix:  m.Destination.ValueString(),
	}
}
