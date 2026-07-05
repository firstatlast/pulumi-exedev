// Command pulumi-resource-exedev is the exe.dev Pulumi provider plugin.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/firstatlast/pulumi-exedev/provider/exedev"
)

// Version is the provider version. It is overridden at build time via
// -ldflags "-X main.Version=x.y.z".
var Version = "0.1.0"

func main() {
	provider, err := exedev.NewProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build provider: %s\n", err)
		os.Exit(1)
	}
	if err := provider.Run(context.Background(), exedev.Name, Version); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
