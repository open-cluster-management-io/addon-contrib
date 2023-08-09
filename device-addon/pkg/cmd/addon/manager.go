package addon

import (
	"open-cluster-management-io/addon-contrib/device-addon/pkg/addon/hub"

	"github.com/spf13/cobra"
	"k8s.io/component-base/version"
	"open-cluster-management.io/addon-framework/pkg/cmd/factory"
)

func NewManagerCommand() *cobra.Command {
	cmd := factory.NewControllerCommandConfig("device-addon-manager", version.Get(), hub.Run).NewCommand()
	cmd.Use = "manager"
	cmd.Short = "Start the addon manager"

	return cmd
}
