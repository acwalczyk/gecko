package placement

import (
	"fmt"
	"os"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	privatev1 "github.com/openshift-online/gecko/platform-api/api/private/v1"

	placement "github.com/openshift-online/gecko/controllers/placement"
	"github.com/openshift-online/gecko/controllers/util/setup"
)

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// NewCommand returns the placement subcommand.
func NewCommand(rf *setup.RootFlags) *cobra.Command {
	var candidateNames, baseDomains []string
	var smProject, maestroHTTPAddr string

	cmd := &cobra.Command{
		Use:   "placement",
		Short: "Run the placement controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			if v := envOr("SECRETMANAGER_PROJECT", ""); v != "" && !cmd.Flags().Changed("secretmanager-project") {
				smProject = v
			}
			if v := envOr("MAESTRO_HTTP_ADDR", ""); v != "" && !cmd.Flags().Changed("maestro-http-addr") {
				maestroHTTPAddr = v
			}

			ctx := cmd.Context()

			log, err := rf.NewLogger("placement-controller")
			if err != nil {
				return fmt.Errorf("create logger: %w", err)
			}

			var selector placement.Selector
			var candidates []placement.Candidate

			if smProject != "" {
				smClient, err := secretmanager.NewClient(ctx)
				if err != nil {
					return fmt.Errorf("create secret manager client: %w", err)
				}
				defer smClient.Close() //nolint:errcheck
				selector = placement.NewDynamicSelector(smClient, smProject, maestroHTTPAddr)
			} else {
				candidates = make([]placement.Candidate, 0, len(candidateNames))
				for i, name := range candidateNames {
					c := placement.Candidate{Name: name}
					if i < len(baseDomains) {
						c.BaseDomains = []string{baseDomains[i]}
					}
					candidates = append(candidates, c)
				}
				selector = placement.NewRoundRobinSelector()
			}

			scheme := setup.NewScheme()
			mgr, err := rf.NewManager(scheme, log)
			if err != nil {
				return fmt.Errorf("create manager: %w", err)
			}

			rec := placement.NewReconciler(selector, candidates, log, mgr.GetClient())

			if err := ctrl.NewControllerManagedBy(mgr).
				For(&privatev1.Cluster{}).
				WithOptions(rf.ControllerOpts()).
				Complete(rec); err != nil {
				return fmt.Errorf("setup controller: %w", err)
			}

			return mgr.Start(ctx)
		},
	}

	cmd.Flags().StringSliceVar(&candidateNames, "candidates", nil, "MC names (comma-separated); ignored when --secretmanager-project is set")
	cmd.Flags().StringSliceVar(&baseDomains, "base-domains", nil, "Base domains per MC, paired with --candidates")
	cmd.Flags().StringVar(&smProject, "secretmanager-project", "", "GCP project for Secret Manager MC/DNS discovery [$SECRETMANAGER_PROJECT]; enables dynamic selector")
	cmd.Flags().StringVar(&maestroHTTPAddr, "maestro-http-addr", "http://maestro.hyperfleet.svc.cluster.local:8000", "Maestro HTTP API URL for consumer discovery [$MAESTRO_HTTP_ADDR]")

	return cmd
}
