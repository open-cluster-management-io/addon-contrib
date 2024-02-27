package hub

import (
	"github.com/spf13/cobra"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"open-cluster-management.io/addon-contrib/clusternet-addon/pkg/hub"
	"open-cluster-management.io/addon-contrib/clusternet-addon/pkg/version"
)

func NewController() *cobra.Command {
	addOnControllerOptions := hub.NewAddOnControllerOptions()
	cmdConfig := controllercmd.
		NewControllerCommandConfig("clusternet-addon-controller", version.Get(), addOnControllerOptions.RunControllerManager)

	cmd := cmdConfig.NewCommand()
	cmd.Use = "controller"
	cmd.Short = "Start the clusternet add-on controller"

	flags := cmd.Flags()
	flags.BoolVar(&cmdConfig.DisableLeaderElection, "disable-leader-election", false, "Disable leader election for the agent.")

	return cmd
}
