package exedev

import (
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Name is the provider (package) name. It must match the plugin binary name
// pulumi-resource-<Name>.
const Name = "exedev"

// NewProvider builds the exe.dev provider.
func NewProvider() (p.Provider, error) {
	return infer.NewProviderBuilder().
		WithNamespace("firstatlast").
		WithDisplayName("exe.dev").
		WithDescription("A Pulumi provider for managing exe.dev VMs.").
		WithHomepage("https://exe.dev").
		WithRepository("https://github.com/firstatlast/pulumi-exedev").
		WithPublisher("firstatlast").
		WithPluginDownloadURL("github://api.github.com/firstatlast/pulumi-exedev").
		WithGoImportPath("github.com/firstatlast/pulumi-exedev/sdk/go/exedev").
		WithLicense("Apache-2.0").
		WithKeywords("exe.dev", "vm", "compute", "category/cloud").
		WithConfig(infer.Config(&Config{})).
		WithResources(
			infer.Resource(&Vm{}),
		).
		Build()
}
