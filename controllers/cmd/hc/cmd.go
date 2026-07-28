package hc

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	privatev1 "github.com/openshift-online/gecko/platform-api/api/private/v1"

	maestroclient "github.com/openshift-online/gecko/controllers/client/maestro"
	maestrotransport "github.com/openshift-online/gecko/controllers/client/transport/maestro"
	hcadapter "github.com/openshift-online/gecko/controllers/hc"
	"github.com/openshift-online/gecko/controllers/rootflags"
)

// maestroFlags holds Maestro-related flags.
type maestroFlags struct {
	grpcAddr string
	httpAddr string
	sourceID string
	clientID string
	insecure bool
}

// NewCommand returns the hc subcommand.
func NewCommand(rf *rootflags.RootFlags) *cobra.Command {
	mf := &maestroFlags{}

	cmd := &cobra.Command{
		Use:   "hc",
		Short: "Run the hosted-cluster (hc) adapter",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			log, err := rf.NewLogger("hc-adapter")
			if err != nil {
				return fmt.Errorf("create logger: %w", err)
			}

			mwc, err := maestroclient.NewMaestroClient(ctx, &maestroclient.Config{
				MaestroServerAddr: mf.httpAddr,
				GRPCServerAddr:    mf.grpcAddr,
				SourceID:          mf.sourceID,
				Insecure:          mf.insecure,
			}, log)
			if err != nil {
				return fmt.Errorf("create maestro client: %w", err)
			}
			defer mwc.Close() //nolint:errcheck

			transport := maestrotransport.New(mwc, mf.sourceID, log)

			scheme := rootflags.NewScheme()
			mgr, err := rf.NewManager(scheme, log)
			if err != nil {
				return fmt.Errorf("create manager: %w", err)
			}

			rec := hcadapter.New(transport, log, mgr.GetClient())

			if err := ctrl.NewControllerManagedBy(mgr).
				For(&privatev1.Cluster{}).
				WithOptions(rf.ControllerOpts()).
				Complete(rec); err != nil {
				return fmt.Errorf("setup controller: %w", err)
			}

			return mgr.Start(ctx)
		},
	}

	cmd.Flags().StringVar(&mf.grpcAddr, "maestro-grpc-addr", "maestro-grpc.hyperfleet.svc.cluster.local:8090", "Maestro gRPC server address")
	cmd.Flags().StringVar(&mf.httpAddr, "maestro-http-addr", "http://maestro.hyperfleet.svc.cluster.local:8000", "Maestro HTTP API server address")
	cmd.Flags().StringVar(&mf.sourceID, "maestro-source-id", "hc-adapter", "Maestro source ID")
	cmd.Flags().StringVar(&mf.clientID, "maestro-client-id", "hc-adapter-client", "Maestro client ID")
	cmd.Flags().BoolVar(&mf.insecure, "maestro-insecure", true, "Disable TLS verification for Maestro connections")

	return cmd
}
