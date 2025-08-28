package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func NameValidator() validator.String {
	return stringvalidator.RegexMatches(
		regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`),
		"Must start with a lowercase letter, contain only lowercase letters, digits or hyphens, end with a letter or digit, and be at most 63 characters long",
	)
}

type Base64Validator struct{}

func (v Base64Validator) Description(ctx context.Context) string {
	return "Must be a valid base64 encoded string"
}

func (v Base64Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v Base64Validator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid Base64 Encoded String",
			fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
		)
	}
}

type ProtocolValidator struct{}

func (v ProtocolValidator) allowedProtocols() []string {
	return []string{"tcp", "udp"}
}

func (v ProtocolValidator) Description(ctx context.Context) string {
	return fmt.Sprintf(
		"Must be one of the following well known protocol strings (%s)",
		strings.Join(v.allowedProtocols(), ", "),
	)
}

func (v ProtocolValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v ProtocolValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	for _, protocol := range v.allowedProtocols() {
		if value == protocol {
			return
		}
	}

	response.Diagnostics.AddAttributeError(
		request.Path,
		"Invalid IP Protocol",
		fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
	)
}

type PortsValidator struct{}

func (v PortsValidator) Description(ctx context.Context) string {
	return "Must be a valid port number (0-65535) or a port range (e.g., 80-443)"
}

func (v PortsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v PortsValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()
	ports := strings.Split(value, "-")

	if len(ports) > 2 {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid Port Format",
			fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
		)
		return
	}

	for _, port := range ports {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 0 || portNumber > 65535 {
			response.Diagnostics.AddAttributeError(
				request.Path,
				"Invalid Port Number",
				fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
			)
			return
		}
	}
}

type CIDRValidator struct{}

func (v CIDRValidator) Description(ctx context.Context) string {
	return "Must be a valid CIDR notation"
}

func (v CIDRValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v CIDRValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	if _, _, err := net.ParseCIDR(value); err != nil {
		response.Diagnostics.AddAttributeError(
			request.Path,
			"Invalid CIDR Notation",
			fmt.Sprintf("Attribute %s %s, got: %s", request.Path, v.Description(ctx), value),
		)
		return
	}
}
