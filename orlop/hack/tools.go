//go:build tools

package hack

// Tool dependencies -- "go mod tidy" only keeps go.sum entries for packages
// that are transitively imported from a .go file. Listing the tool commands
// here ensures their transitive dependencies (e.g., k8s.io/gengo/v2) stay
// in go.sum so that "go run" in the Makefile works.
import (
	_ "k8s.io/kube-openapi/cmd/openapi-gen"
)
