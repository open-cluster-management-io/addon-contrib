package hub

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/utils/clock"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/version"
	commonoptions "open-cluster-management.io/ocm/pkg/common/options"
)

func NewController() *cobra.Command {
	opts := commonoptions.NewOptions()
	cmdConfig := opts.
		NewControllerCommandConfig("kueue-addon-controller", version.Get(), hub.RunControllerManager, clock.RealClock{})
	cmd := cmdConfig.NewCommandWithContext(context.TODO())
	cmd.Use = "hub"
	cmd.Short = "Start the Kueue Add-On Hub Controller"

	flags := cmd.Flags()
	opts.AddFlags(flags)

	return cmd
}
