// Package shim exposes the provider factory outside this module, so the
// Pulumi bridge (github.com/asyntai/pulumi-asyntai) can wrap the same
// provider code. Go does not allow other modules to import internal/.
package shim

import (
	"github.com/hashicorp/terraform-plugin-framework/provider"

	asyntai "github.com/asyntai/terraform-provider-asyntai/internal/provider"
)

// NewProvider returns the Asyntai terraform-plugin-framework provider. It
// sends "pulumi-asyntai/<version>" as its User-Agent, so the Asyntai API can
// tell Pulumi traffic from Terraform traffic.
func NewProvider(version string) provider.Provider {
	return asyntai.NewWithProduct("pulumi-asyntai", version)()
}
