package device

import (
	"context"

	"github.com/spf13/cobra"

	"k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/device"
)

func NewDriverCommand() *cobra.Command {
	o := device.NewDriverAgentOptions()
	ctx := context.TODO()
	cmd := &cobra.Command{
		Use:   "driver",
		Short: "Start the driver agent",
		Run: func(cmd *cobra.Command, args []string) {
			logs.InitLogs()

			shutdownCtx, cancel := context.WithCancel(ctx)
			shutdownHandler := server.SetupSignalHandler()
			go func() {
				defer cancel()
				<-shutdownHandler
			}()

			ctx, terminate := context.WithCancel(shutdownCtx)
			defer terminate()

			if err := o.RunDriverAgent(ctx); err != nil {
				klog.Fatal(err)
			}
		},
	}

	o.AddFlags(cmd.Flags())

	return cmd
}
