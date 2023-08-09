package addon

import (
	"github.com/spf13/cobra"

	"k8s.io/component-base/version"

	"open-cluster-management.io/addon-framework/pkg/cmd/factory"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/addon/spoke"
)

func NewAgentCommand() *cobra.Command {
	o := spoke.NewAgentOptions()
	cmd := factory.NewControllerCommandConfig("device-addon-agent", version.Get(), o.RunAgent).NewCommand()
	cmd.Use = "agent"
	cmd.Short = "Start the addon agent"

	o.AddFlags(cmd.Flags())

	return cmd
}
