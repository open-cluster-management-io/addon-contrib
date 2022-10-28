package addonmanager

import (
	"context"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

// ControllerFlags provides the "normal" controller flags
type ControllerFlags struct {
	// KubeConfigFile points to a kubeconfig file if you don't want to use the in cluster config
	KubeConfigFile string
}

func (f *ControllerFlags) NewControllerCommand() *cobra.Command {
	ctx := context.TODO()
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "helloworld example addon",
		Run: func(cmd *cobra.Command, args []string) {
			if err := f.StartController(ctx); err != nil {
				klog.Fatal(err)
			}
		},
	}

	f.AddFlags(cmd)

	return cmd
}

// AddFlags register and binds the default flags
func (f *ControllerFlags) AddFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	// This command only supports reading from config

	flags.StringVar(&f.KubeConfigFile, "kubeconfig", f.KubeConfigFile, "Location of the master configuration file to run from.")
	cmd.MarkFlagFilename("kubeconfig", "kubeconfig")
}

// StartController runs the controller. This is the recommend entrypoint when you don't need
// to customize the builder.
func (c *ControllerFlags) StartController(ctx context.Context) error {
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", c.KubeConfigFile)
	if err != nil {
		return err
	}

	manager, err := NewAddonManager(kubeConfig)
	if err != nil {
		return err
	}

	err = manager.Start(ctx)
	if err != nil {
		return err
	}
	<-ctx.Done()

	return nil
}
