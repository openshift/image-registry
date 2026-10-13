/*
This command is used to run the Image Registry tests extension for OpenShift.
It registers the image-registry tests with the OpenShift Tests Extension framework
and provides a command-line interface to execute them.
For further information, please refer to the documentation at:
https://github.com/openshift-eng/openshift-tests-extension/blob/main/cmd/example-tests/main.go
*/
package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/component-base/cli"

	otecmd "github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	oteextension "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	"github.com/openshift/image-registry/pkg/version"

	"k8s.io/klog/v2"
)

func main() {
	command := newOperatorTestCommand(context.Background())
	code := cli.Run(command)
	os.Exit(code)
}

func newOperatorTestCommand(ctx context.Context) *cobra.Command {
	registry := prepareOperatorTestsRegistry()

	cmd := &cobra.Command{
		Use:   "dockerregistry-tests-ext",
		Short: "A binary used to run image-registry tests as part of OTE.",
		Run: func(cmd *cobra.Command, args []string) {
			// no-op, logic is provided by the OTE framework
			if err := cmd.Help(); err != nil {
				klog.Fatal(err)
			}
		},
	}

	v := version.Get()
	if len(v.GitVersion) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v.GitVersion
	}

	cmd.AddCommand(otecmd.DefaultExtensionCommands(registry)...)

	return cmd
}

// prepareOperatorTestsRegistry creates the OTE registry for this operator.
//
// Note:
//
// This method must be called before adding the registry to the OTE framework.
func prepareOperatorTestsRegistry() *oteextension.Registry {
	registry := oteextension.NewRegistry()
	extension := oteextension.NewExtension("openshift", "payload", "image-registry")

	registry.Register(extension)
	return registry
}
