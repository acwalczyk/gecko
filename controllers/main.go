package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	cmdhc "github.com/openshift-online/gecko/controllers/cmd/hc"
	cmdnodepool "github.com/openshift-online/gecko/controllers/cmd/nodepool"
	cmdnodepoolvrresolution "github.com/openshift-online/gecko/controllers/cmd/nodepoolvrresolution"
	cmdplacement "github.com/openshift-online/gecko/controllers/cmd/placement"
	cmdversionresolution "github.com/openshift-online/gecko/controllers/cmd/versionresolution"
	"github.com/openshift-online/gecko/controllers/util/setup"
)

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func main() {
	rf := &setup.RootFlags{}

	root := &cobra.Command{
		Use:   "gecko-controllers",
		Short: "Gecko controllers",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if v := envOr("LOG_LEVEL", ""); v != "" && !cmd.Flags().Changed("log-level") {
				rf.LogLevel = v
			}
			if v := envOr("LOG_FORMAT", ""); v != "" && !cmd.Flags().Changed("log-format") {
				rf.LogFormat = v
			}
			if v := envOr("ORLOP_URL", ""); v != "" && !cmd.Flags().Changed("orlop-url") {
				rf.OrlopURL = v
			}
		},
	}

	root.PersistentFlags().StringVar(&rf.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	root.PersistentFlags().StringVar(&rf.LogFormat, "log-format", "json", "Log format (json, text)")
	root.PersistentFlags().StringVar(&rf.OrlopURL, "orlop-url", "http://hyperfleet-api:8080", "Orlop API server URL [$ORLOP_URL]")
	root.PersistentFlags().IntVar(&rf.Workers, "workers", 10, "Concurrent reconcile goroutines")

	root.AddCommand(
		cmdversionresolution.NewCommand(rf),
		cmdnodepoolvrresolution.NewCommand(rf),
		cmdplacement.NewCommand(rf),
		cmdhc.NewCommand(rf),
		cmdnodepool.NewCommand(rf),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
