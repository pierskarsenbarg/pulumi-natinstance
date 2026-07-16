// Command pulumi-resource-nat is the entrypoint for the nat Pulumi provider.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pierskarsenbarg/pulumi-natinstance/pkg"
	dotnetgen "github.com/pulumi/pulumi-dotnet/pulumi-language-dotnet/v3/codegen"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi-go-provider/middleware/schema"
	gen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	nodejsgen "github.com/pulumi/pulumi/pkg/v3/codegen/nodejs"
	pythongen "github.com/pulumi/pulumi/pkg/v3/codegen/python"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

var (
	Version      = "0.0.1"
	providerName = "natinstance"
)

func main() {
	err := p.RunProvider(context.Background(), providerName, Version, provider())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(1)
	}
}

func provider() p.Provider {
	return infer.Provider(infer.Options{
		Metadata: schema.Metadata{
			DisplayName: providerName,
			Description: "Pulumi Component to create a nat instance",
			LanguageMap: map[string]any{
				"go": gen.GoPackageInfo{
					ImportBasePath: fmt.Sprintf("github.com/pierskarsenbarg/pulumi-%s/sdk/go/%s", providerName, providerName),
				},
				"nodejs": nodejsgen.NodePackageInfo{
					PackageName: fmt.Sprintf("@pierskarsenbarg/%s", providerName),
					Dependencies: map[string]string{
						"@pulumi/pulumi": "^3.0.0",
						"@pulumi/aws":    "^7.0.0",
					},
					DevDependencies: map[string]string{
						"@types/node": "^10.0.0", // so we can access strongly typed node definitions.
						"@types/mime": "^2.0.0",
					},
				},
				"csharp": dotnetgen.CSharpPackageInfo{
					RootNamespace: "PiersKarsenbarg",
					PackageReferences: map[string]string{
						"Pulumi":     "3.*",
						"Pulumi.Aws": "7.*",
					},
				},
				"python": pythongen.PackageInfo{
					Requires: map[string]string{
						"pulumi":     ">=3.0.0,<4.0.0",
						"pulumi-aws": ">=7.0.0,<8.0.0",
					},
					PackageName: fmt.Sprintf("pierskarsenbarg_pulumi_%s", providerName),
				},
			},
			PluginDownloadURL: fmt.Sprintf("github://api.github.com/pierskarsenbarg/pulumi-%s", providerName),
			Publisher:         "Piers Karsenbarg",
		},
		ModuleMap: map[tokens.ModuleName]tokens.ModuleName{
			"pkg": "index", // required because the folder with everything in is "pkg"
		},
		Components: []infer.InferredComponent{
			infer.Component(&pkg.NatInstance{}),
		},
	})
}
