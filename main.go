package main

import (
	"flag"
	"time"

	"github.com/better0332/resbalancer/balancer"
	"github.com/better0332/resbalancer/clientset"
	"github.com/better0332/resbalancer/signal"
	"github.com/spf13/cobra"
	"k8s.io/klog"
)

var (
	rootCmd = &cobra.Command{
		Long:    "K8S Resource Balancer Application.",
		Version: "0.0.1",
		Run: func(cmd *cobra.Command, args []string) {
			if ratio < 1 {
				klog.Fatalf("ratio %v too small (<1)", ratio)
			}

			stopCh := signal.SetupSignalHandler()

			clientset, err := clientset.GetClient(kubeconfig)
			if err != nil {
				klog.Fatalf("get k8s clientset error: %v", err)
			}

			b := balancer.NewBalancer(clientset, recyclePeriod, ratio)

			klog.Info("balancer running")
			b.Run(stopCh)
			klog.Info("balancer stopped")
		},
	}

	kubeconfig    string
	recyclePeriod time.Duration
	ratio         float64
)

func init() {
	klog.InitFlags(nil)

	rootCmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "c", "", "Kube config path. Only required if out-of-cluster.")
	rootCmd.Flags().DurationVarP(&recyclePeriod, "recycle_period", "p", 2*time.Minute, "Recycle period.")
	rootCmd.Flags().Float64VarP(&ratio, "ratio", "r", 2.5, "sensitivity ratio [1,n]")
}

func main() {
	rootCmd.Execute()
}
