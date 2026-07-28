package nodepoolvrresolution

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	privatev1 "github.com/openshift-online/gecko/platform-api/api/private/v1"

	"github.com/openshift-online/gecko/controllers/nodepoolvrresolution"
	"github.com/openshift-online/gecko/controllers/rootflags"
	"github.com/openshift-online/gecko/controllers/versionresolution"
)

// NewCommand returns the nodepool-vr subcommand.
func NewCommand(rf *rootflags.RootFlags) *cobra.Command {
	var cincinnatiURL, arch string

	cmd := &cobra.Command{
		Use:   "nodepool-vr",
		Short: "Run the nodepool version-resolution adapter",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			log, err := rf.NewLogger("nodepool-vr-adapter")
			if err != nil {
				return fmt.Errorf("create logger: %w", err)
			}

			scheme := rootflags.NewScheme()
			mgr, err := rf.NewManager(scheme, log)
			if err != nil {
				return fmt.Errorf("create manager: %w", err)
			}

			cinClient := versionresolution.NewCincinnatiClient(cincinnatiURL, arch)
			rec := nodepoolvrresolution.NewReconciler(cinClient, log, mgr.GetClient())

			if err := ctrl.NewControllerManagedBy(mgr).
				For(&privatev1.NodePool{}).
				WithOptions(rf.ControllerOpts()).
				Complete(rec); err != nil {
				return fmt.Errorf("setup controller: %w", err)
			}

			return mgr.Start(ctx)
		},
	}

	cmd.Flags().StringVar(&cincinnatiURL, "cincinnati-url", "https://api.openshift.com/api/upgrades_info/v1/graph", "Cincinnati API URL")
	cmd.Flags().StringVar(&arch, "arch", "amd64", "CPU architecture for Cincinnati query")

	return cmd
}
