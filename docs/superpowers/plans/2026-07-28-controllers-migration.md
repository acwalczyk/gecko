# Controllers Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate five adapter controllers from `github.com/openshift-hyperfleet/hyperfleet-adapters-go` into `gecko/controllers/` as a new Go module, updating all type references from `github.com/thetechnick/orlop-gcp-hcp/api/private/v1` to `github.com/openshift-online/gecko/platform-api/api/private/v1`.

**Architecture:** New top-level `controllers/` module (`github.com/openshift-online/gecko/controllers`) with a single binary `gecko-controllers` built from `main.go`. Controllers live at the top level as flat packages. Shared client code goes in `client/`, generic utilities in `util/`. The module references `platform-api` types via a `replace` directive.

**Tech Stack:** Go 1.26, controller-runtime v0.24.1, cobra, Maestro HTTP/gRPC client, OCM SDK

---

## File Map

**Source repo:** `/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go`
**Target module:** `gecko/controllers/`

| Source | Target |
|--------|--------|
| `cmd/main.go` | `main.go` + `cmd/*/cmd.go` (split by subcommand) |
| `internal/adapters/versionresolution/cincinnati.go` | `versionresolution/cincinnati.go` |
| `internal/adapters/versionresolution/reconciler.go` | `versionresolution/versionresolution_controller.go` |
| `internal/adapters/versionresolution/reconciler_test.go` | `versionresolution/versionresolution_controller_test.go` |
| `internal/adapters/nodepoolvrresolution/reconciler.go` | `nodepoolvrresolution/nodepoolvrresolution_controller.go` |
| `internal/adapters/nodepoolvrresolution/reconciler_test.go` | `nodepoolvrresolution/nodepoolvrresolution_controller_test.go` |
| `internal/adapters/placement/selector.go` | `placement/selector.go` |
| `internal/adapters/placement/dynamic_selector.go` | `placement/dynamic_selector.go` |
| `internal/adapters/placement/dynamic_selector_test.go` | `placement/dynamic_selector_test.go` |
| `internal/adapters/placement/reconciler.go` | `placement/placement_controller.go` |
| `internal/adapters/placement/reconciler_test.go` | `placement/placement_controller_test.go` |
| `internal/adapters/hc/reconciler.go` | `hc/hc_controller.go` |
| `internal/adapters/hc/reconciler_test.go` | `hc/hc_controller_test.go` |
| `internal/adapters/hc/manifest/manifestwork.go` | `hc/manifest/manifestwork.go` |
| `internal/adapters/hc/manifest/manifestwork_test.go` | `hc/manifest/manifestwork_test.go` |
| `internal/adapters/nodepool/reconciler.go` | `nodepool/nodepool_controller.go` |
| `internal/adapters/nodepool/reconciler_test.go` | `nodepool/nodepool_controller_test.go` |
| `internal/adapters/nodepool/manifest/manifestwork.go` | `nodepool/manifest/manifestwork.go` |
| `internal/adapters/nodepool/manifest/manifestwork_test.go` | `nodepool/manifest/manifestwork_test.go` |
| `internal/maestroclient/` (all files) | `client/maestro/` |
| `internal/transport/interface.go` | `client/transport/interface.go` |
| `internal/transport/maestro/client.go` | `client/transport/maestro/client.go` |
| `internal/transport/mock/client.go` | `client/transport/mock/client.go` |
| `internal/transportclient/interface.go` | `client/transportclient/interface.go` |
| `internal/transportclient/types.go` | `client/transportclient/types.go` |
| `internal/manifest/generation.go` | `util/manifest/generation.go` |
| `internal/manifest/generation_test.go` | `util/manifest/generation_test.go` |
| `internal/conditions/conditions.go` | `util/conditions/conditions.go` |
| `pkg/constants/constants.go` | `util/constants/constants.go` |
| `pkg/errors/*.go` (5 files) | `util/errors/` |
| `pkg/logger/*.go` (3 files) | `util/logger/` |
| `pkg/version/version.go` | `util/version/version.go` |

## Import Path Substitutions

Every `.go` file copied needs these substitutions applied (in this order):

| Old import | New import |
|------------|------------|
| `github.com/thetechnick/orlop-gcp-hcp/api/private/v1` | `github.com/openshift-online/gecko/platform-api/api/private/v1` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/versionresolution` | `github.com/openshift-online/gecko/controllers/versionresolution` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/nodepoolvrresolution` | `github.com/openshift-online/gecko/controllers/nodepoolvrresolution` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/placement` | `github.com/openshift-online/gecko/controllers/placement` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/hc` | `github.com/openshift-online/gecko/controllers/hc` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/nodepool` | `github.com/openshift-online/gecko/controllers/nodepool` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/maestroclient` | `github.com/openshift-online/gecko/controllers/client/maestro` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport/maestro` | `github.com/openshift-online/gecko/controllers/client/transport/maestro` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport` | `github.com/openshift-online/gecko/controllers/client/transport` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transportclient` | `github.com/openshift-online/gecko/controllers/client/transportclient` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/manifest` | `github.com/openshift-online/gecko/controllers/util/manifest` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/conditions` | `github.com/openshift-online/gecko/controllers/util/conditions` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/constants` | `github.com/openshift-online/gecko/controllers/util/constants` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors` | `github.com/openshift-online/gecko/controllers/util/errors` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger` | `github.com/openshift-online/gecko/controllers/util/logger` |
| `github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/version` | `github.com/openshift-online/gecko/controllers/util/version` |

---

## Task 1: Create Feature Branch

**Files:** none

- [ ] **Step 1: Create and switch to feature branch**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git checkout -b feat/controllers-migration
```

Expected: `Switched to a new branch 'feat/controllers-migration'`

---

## Task 2: Scaffold the controllers/ Module

**Files:**
- Create: `controllers/go.mod`
- Create: `controllers/main.go` (skeleton only — wired up in Task 11)

- [ ] **Step 1: Create the directory structure**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
mkdir -p controllers/{cmd/{versionresolution,nodepoolvrresolution,placement,hc,nodepool},versionresolution,nodepoolvrresolution,placement,hc/manifest,nodepool/manifest,client/{maestro,transport/{maestro,mock},transportclient},util/{constants,errors,logger,manifest,conditions,version}}
```

- [ ] **Step 2: Create go.mod**

Create `controllers/go.mod` with this content:

```
module github.com/openshift-online/gecko/controllers

go 1.26.4

require (
	cloud.google.com/go/secretmanager v1.20.0
	github.com/go-logr/logr v1.4.3
	github.com/openshift-online/gecko/platform-api v0.0.0
	github.com/openshift-online/maestro v0.0.0-20260202062555-48b47506a254
	github.com/openshift-online/ocm-sdk-go v0.1.493
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/otel/trace v1.43.0
	google.golang.org/api v0.274.0
	k8s.io/apimachinery v0.36.2
	k8s.io/client-go v0.36.2
	open-cluster-management.io/api v1.2.0
	open-cluster-management.io/sdk-go v1.2.0
	sigs.k8s.io/controller-runtime v0.24.1
	sigs.k8s.io/yaml v1.6.0
)

replace github.com/openshift-online/gecko/platform-api => ../platform-api
```

- [ ] **Step 3: Create skeleton main.go**

Create `controllers/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "gecko-controllers",
		Short: "Gecko controllers",
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Verify module is recognized**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./...
```

Expected: succeeds (only main.go compiles, no errors)

---

## Task 3: Migrate util/ Packages

**Files:**
- Create: `controllers/util/constants/constants.go`
- Create: `controllers/util/errors/` (5 files)
- Create: `controllers/util/logger/` (3 files)
- Create: `controllers/util/version/version.go`
- Create: `controllers/util/conditions/conditions.go`
- Create: `controllers/util/manifest/generation.go`
- Create: `controllers/util/manifest/generation_test.go`

- [ ] **Step 1: Copy util package files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/pkg/constants/constants.go $DST/util/constants/
cp $SRC/pkg/errors/*.go $DST/util/errors/
cp $SRC/pkg/logger/*.go $DST/util/logger/
cp $SRC/pkg/version/version.go $DST/util/version/
cp $SRC/internal/conditions/conditions.go $DST/util/conditions/
cp $SRC/internal/manifest/generation.go $DST/util/manifest/
cp $SRC/internal/manifest/generation_test.go $DST/util/manifest/
```

- [ ] **Step 2: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/util -name "*.go" -exec sed -i \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/constants|github.com/openshift-online/gecko/controllers/util/constants|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/version|github.com/openshift-online/gecko/controllers/util/version|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/conditions|github.com/openshift-online/gecko/controllers/util/conditions|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/manifest|github.com/openshift-online/gecko/controllers/util/manifest|g' \
  {} \;
```

- [ ] **Step 3: Run go mod tidy to fetch indirect deps**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go mod tidy 2>&1 | head -20
```

Expected: downloads dependencies, no errors. If any package is not found, check that the package name still exists in the dependency (dependency may have been updated).

- [ ] **Step 4: Build util packages**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./util/...
```

Expected: no errors

- [ ] **Step 5: Run util tests**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go test ./util/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/go.mod controllers/main.go controllers/util/
git commit -m "feat(controllers): scaffold module and migrate util packages"
```

---

## Task 4: Migrate client/maestro

**Files:**
- Create: `controllers/client/maestro/client.go`
- Create: `controllers/client/maestro/client_test.go`
- Create: `controllers/client/maestro/interface.go`
- Create: `controllers/client/maestro/ocm_logger_adapter.go`
- Create: `controllers/client/maestro/operations.go`
- Create: `controllers/client/maestro/operations_test.go`

- [ ] **Step 1: Copy maestroclient files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/internal/maestroclient/client.go $DST/client/maestro/
cp $SRC/internal/maestroclient/client_test.go $DST/client/maestro/
cp $SRC/internal/maestroclient/interface.go $DST/client/maestro/
cp $SRC/internal/maestroclient/ocm_logger_adapter.go $DST/client/maestro/
cp $SRC/internal/maestroclient/operations.go $DST/client/maestro/
cp $SRC/internal/maestroclient/operations_test.go $DST/client/maestro/
```

- [ ] **Step 2: Update package declaration in all copied files**

The source package is named `maestroclient`. The target package must be `maestro`. Update the declaration in all files:

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

sed -i 's/^package maestroclient$/package maestro/' $DST/client/maestro/*.go
```

- [ ] **Step 3: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/client/maestro -name "*.go" -exec sed -i \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/maestroclient|github.com/openshift-online/gecko/controllers/client/maestro|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  {} \;
```

- [ ] **Step 4: Build**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./client/maestro/...
```

Expected: no errors. If there are compilation errors, read the error, find the offending import, and apply the correct substitution from the Import Path Substitutions table above.

- [ ] **Step 5: Run tests**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go test ./client/maestro/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/client/maestro/
git commit -m "feat(controllers): migrate maestro client"
```

---

## Task 5: Migrate client/transport and client/transportclient

**Files:**
- Create: `controllers/client/transport/interface.go`
- Create: `controllers/client/transport/maestro/client.go`
- Create: `controllers/client/transport/mock/client.go`
- Create: `controllers/client/transportclient/interface.go`
- Create: `controllers/client/transportclient/types.go`

- [ ] **Step 1: Copy transport files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/internal/transport/interface.go $DST/client/transport/
cp $SRC/internal/transport/maestro/client.go $DST/client/transport/maestro/
cp $SRC/internal/transport/mock/client.go $DST/client/transport/mock/
cp $SRC/internal/transportclient/interface.go $DST/client/transportclient/
cp $SRC/internal/transportclient/types.go $DST/client/transportclient/
```

- [ ] **Step 2: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/client/transport $DST/client/transportclient -name "*.go" -exec sed -i \
  -e 's|github.com/thetechnick/orlop-gcp-hcp/api/private/v1|github.com/openshift-online/gecko/platform-api/api/private/v1|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/maestroclient|github.com/openshift-online/gecko/controllers/client/maestro|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport/maestro|github.com/openshift-online/gecko/controllers/client/transport/maestro|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport|github.com/openshift-online/gecko/controllers/client/transport|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transportclient|github.com/openshift-online/gecko/controllers/client/transportclient|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/manifest|github.com/openshift-online/gecko/controllers/util/manifest|g' \
  {} \;
```

- [ ] **Step 3: Build**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./client/...
```

Expected: no errors

- [ ] **Step 4: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/client/transport/ controllers/client/transportclient/
git commit -m "feat(controllers): migrate transport client"
```

---

## Task 6: Migrate versionresolution Controller

**Files:**
- Create: `controllers/versionresolution/cincinnati.go`
- Create: `controllers/versionresolution/versionresolution_controller.go`
- Create: `controllers/versionresolution/versionresolution_controller_test.go`

- [ ] **Step 1: Copy and rename files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/internal/adapters/versionresolution/cincinnati.go $DST/versionresolution/
cp $SRC/internal/adapters/versionresolution/reconciler.go $DST/versionresolution/versionresolution_controller.go
cp $SRC/internal/adapters/versionresolution/reconciler_test.go $DST/versionresolution/versionresolution_controller_test.go
```

- [ ] **Step 2: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/versionresolution -name "*.go" -exec sed -i \
  -e 's|github.com/thetechnick/orlop-gcp-hcp/api/private/v1|github.com/openshift-online/gecko/platform-api/api/private/v1|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/versionresolution|github.com/openshift-online/gecko/controllers/versionresolution|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/conditions|github.com/openshift-online/gecko/controllers/util/conditions|g' \
  {} \;
```

- [ ] **Step 3: Build**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./versionresolution/...
```

Expected: no errors

- [ ] **Step 4: Run tests**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go test ./versionresolution/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/versionresolution/
git commit -m "feat(controllers): migrate versionresolution controller"
```

---

## Task 7: Migrate nodepoolvrresolution Controller

**Files:**
- Create: `controllers/nodepoolvrresolution/nodepoolvrresolution_controller.go`
- Create: `controllers/nodepoolvrresolution/nodepoolvrresolution_controller_test.go`

- [ ] **Step 1: Copy and rename files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/internal/adapters/nodepoolvrresolution/reconciler.go $DST/nodepoolvrresolution/nodepoolvrresolution_controller.go
cp $SRC/internal/adapters/nodepoolvrresolution/reconciler_test.go $DST/nodepoolvrresolution/nodepoolvrresolution_controller_test.go
```

- [ ] **Step 2: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/nodepoolvrresolution -name "*.go" -exec sed -i \
  -e 's|github.com/thetechnick/orlop-gcp-hcp/api/private/v1|github.com/openshift-online/gecko/platform-api/api/private/v1|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/versionresolution|github.com/openshift-online/gecko/controllers/versionresolution|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/nodepoolvrresolution|github.com/openshift-online/gecko/controllers/nodepoolvrresolution|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/conditions|github.com/openshift-online/gecko/controllers/util/conditions|g' \
  {} \;
```

- [ ] **Step 3: Build and test**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./nodepoolvrresolution/... && go test ./nodepoolvrresolution/...
```

Expected: no errors, PASS

- [ ] **Step 4: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/nodepoolvrresolution/
git commit -m "feat(controllers): migrate nodepoolvrresolution controller"
```

---

## Task 8: Migrate placement Controller

**Files:**
- Create: `controllers/placement/selector.go`
- Create: `controllers/placement/dynamic_selector.go`
- Create: `controllers/placement/dynamic_selector_test.go`
- Create: `controllers/placement/placement_controller.go`
- Create: `controllers/placement/placement_controller_test.go`

- [ ] **Step 1: Copy and rename files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/internal/adapters/placement/selector.go $DST/placement/
cp $SRC/internal/adapters/placement/dynamic_selector.go $DST/placement/
cp $SRC/internal/adapters/placement/dynamic_selector_test.go $DST/placement/
cp $SRC/internal/adapters/placement/reconciler.go $DST/placement/placement_controller.go
cp $SRC/internal/adapters/placement/reconciler_test.go $DST/placement/placement_controller_test.go
```

- [ ] **Step 2: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/placement -name "*.go" -exec sed -i \
  -e 's|github.com/thetechnick/orlop-gcp-hcp/api/private/v1|github.com/openshift-online/gecko/platform-api/api/private/v1|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/placement|github.com/openshift-online/gecko/controllers/placement|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/maestroclient|github.com/openshift-online/gecko/controllers/client/maestro|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/conditions|github.com/openshift-online/gecko/controllers/util/conditions|g' \
  {} \;
```

- [ ] **Step 3: Build and test**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./placement/... && go test ./placement/...
```

Expected: no errors, PASS

- [ ] **Step 4: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/placement/
git commit -m "feat(controllers): migrate placement controller"
```

---

## Task 9: Migrate hc Controller

**Files:**
- Create: `controllers/hc/hc_controller.go`
- Create: `controllers/hc/hc_controller_test.go`
- Create: `controllers/hc/manifest/manifestwork.go`
- Create: `controllers/hc/manifest/manifestwork_test.go`

- [ ] **Step 1: Copy and rename files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/internal/adapters/hc/reconciler.go $DST/hc/hc_controller.go
cp $SRC/internal/adapters/hc/reconciler_test.go $DST/hc/hc_controller_test.go
cp $SRC/internal/adapters/hc/manifest/manifestwork.go $DST/hc/manifest/
cp $SRC/internal/adapters/hc/manifest/manifestwork_test.go $DST/hc/manifest/
```

- [ ] **Step 2: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/hc -name "*.go" -exec sed -i \
  -e 's|github.com/thetechnick/orlop-gcp-hcp/api/private/v1|github.com/openshift-online/gecko/platform-api/api/private/v1|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/hc|github.com/openshift-online/gecko/controllers/hc|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport/maestro|github.com/openshift-online/gecko/controllers/client/transport/maestro|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport|github.com/openshift-online/gecko/controllers/client/transport|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transportclient|github.com/openshift-online/gecko/controllers/client/transportclient|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/manifest|github.com/openshift-online/gecko/controllers/util/manifest|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/conditions|github.com/openshift-online/gecko/controllers/util/conditions|g' \
  {} \;
```

- [ ] **Step 3: Build and test**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./hc/... && go test ./hc/...
```

Expected: no errors, PASS

- [ ] **Step 4: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/hc/
git commit -m "feat(controllers): migrate hc controller"
```

---

## Task 10: Migrate nodepool Controller

**Files:**
- Create: `controllers/nodepool/nodepool_controller.go`
- Create: `controllers/nodepool/nodepool_controller_test.go`
- Create: `controllers/nodepool/manifest/manifestwork.go`
- Create: `controllers/nodepool/manifest/manifestwork_test.go`

- [ ] **Step 1: Copy and rename files**

```bash
SRC=/home/cveiga/go/src/github.com/openshift-hyperfleet/hyperfleet-adapters-go
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

cp $SRC/internal/adapters/nodepool/reconciler.go $DST/nodepool/nodepool_controller.go
cp $SRC/internal/adapters/nodepool/reconciler_test.go $DST/nodepool/nodepool_controller_test.go
cp $SRC/internal/adapters/nodepool/manifest/manifestwork.go $DST/nodepool/manifest/
cp $SRC/internal/adapters/nodepool/manifest/manifestwork_test.go $DST/nodepool/manifest/
```

- [ ] **Step 2: Apply import path substitutions**

```bash
DST=/home/cveiga/go/src/github.com/openshift-online/gecko/controllers

find $DST/nodepool -name "*.go" -exec sed -i \
  -e 's|github.com/thetechnick/orlop-gcp-hcp/api/private/v1|github.com/openshift-online/gecko/platform-api/api/private/v1|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/adapters/nodepool|github.com/openshift-online/gecko/controllers/nodepool|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport/maestro|github.com/openshift-online/gecko/controllers/client/transport/maestro|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transport|github.com/openshift-online/gecko/controllers/client/transport|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/transportclient|github.com/openshift-online/gecko/controllers/client/transportclient|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/manifest|github.com/openshift-online/gecko/controllers/util/manifest|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/logger|github.com/openshift-online/gecko/controllers/util/logger|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/pkg/errors|github.com/openshift-online/gecko/controllers/util/errors|g' \
  -e 's|github.com/openshift-hyperfleet/hyperfleet-adapters-go/internal/conditions|github.com/openshift-online/gecko/controllers/util/conditions|g' \
  {} \;
```

- [ ] **Step 3: Build and test**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./nodepool/... && go test ./nodepool/...
```

Expected: no errors, PASS

- [ ] **Step 4: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/nodepool/
git commit -m "feat(controllers): migrate nodepool controller"
```

---

## Task 11: Wire Up main.go and cmd/ Subcommands

**Files:**
- Modify: `controllers/main.go`
- Create: `controllers/cmd/versionresolution/cmd.go`
- Create: `controllers/cmd/nodepoolvrresolution/cmd.go`
- Create: `controllers/cmd/placement/cmd.go`
- Create: `controllers/cmd/hc/cmd.go`
- Create: `controllers/cmd/nodepool/cmd.go`

The existing `cmd/main.go` from hyperfleet-adapters-go has all subcommand logic inline. Split it: each subcommand's `newXxxCmd` function becomes a `NewCommand()` function in its own `cmd/` package.

- [ ] **Step 1: Create cmd/versionresolution/cmd.go**

```go
package versionresolution

import (
	"fmt"

	"github.com/spf13/cobra"
	privatev1 "github.com/openshift-online/gecko/platform-api/api/private/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/openshift-online/gecko/controllers/versionresolution"
	"github.com/openshift-online/gecko/controllers/util/logger"
)

func NewCommand(rf *RootFlags) *cobra.Command {
	var cincinnatiURL, arch string

	cmd := &cobra.Command{
		Use:   "version-resolution",
		Short: "Run the version-resolution controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			log, err := logger.NewLogger(logger.Config{
				Level: rf.LogLevel, Format: rf.LogFormat,
				Output: "stdout", Component: "version-resolution",
			})
			if err != nil {
				return fmt.Errorf("create logger: %w", err)
			}

			scheme, mgr, err := rf.NewManager(log)
			if err != nil {
				return err
			}

			cinClient := versionresolution.NewCincinnatiClient(cincinnatiURL, arch)
			rec := versionresolution.NewReconciler(cinClient, log, mgr.GetClient())

			if err := ctrl.NewControllerManagedBy(mgr).
				For(&privatev1.Cluster{}).
				WithOptions(controller.Options{MaxConcurrentReconciles: rf.Workers}).
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
```

> **Note:** `RootFlags` and its `NewManager` helper are defined in `controllers/main.go` (see Step 6). The pattern repeats for all subcommands — copy the corresponding `newXxxCmd` function from the source `cmd/main.go`, rename it to `NewCommand`, replace `rf *rootFlags` with `rf *RootFlags`, and move it into its own package. Apply the same import substitutions from the Import Path Substitutions table.

- [ ] **Step 2: Create cmd/nodepoolvrresolution/cmd.go**

Follow the same pattern as Step 1, using `nodepoolvrresolution.NewReconciler` watching `privatev1.NodePool`.
Copy from the `newNodepoolVRCmd` function in the source `cmd/main.go`.

- [ ] **Step 3: Create cmd/placement/cmd.go**

Follow the same pattern, using `placement.NewReconciler` watching `privatev1.Cluster`.
Copy from the `newPlacementCmd` function in the source `cmd/main.go`.
Includes Secret Manager client setup and selector logic — copy as-is, applying import substitutions.

- [ ] **Step 4: Create cmd/hc/cmd.go**

Follow the same pattern, using `hc.New` watching `privatev1.Cluster`.
Copy from the `newHCCmd` function in the source `cmd/main.go`.
Includes Maestro client setup — copy as-is, applying import substitutions.
`maestroclient` import → `client/maestro`, `transport/maestro` import → `client/transport/maestro`.

- [ ] **Step 5: Create cmd/nodepool/cmd.go**

Follow the same pattern, using `nodepooladapter.New` watching `privatev1.NodePool`.
Copy from the `newNodepoolCmd` function in the source `cmd/main.go`.

- [ ] **Step 6: Replace controllers/main.go with the full wired version**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	privatev1 "github.com/openshift-online/gecko/platform-api/api/private/v1"

	cmdhc "github.com/openshift-online/gecko/controllers/cmd/hc"
	cmdnodepool "github.com/openshift-online/gecko/controllers/cmd/nodepool"
	cmdnodepoolvrresolution "github.com/openshift-online/gecko/controllers/cmd/nodepoolvrresolution"
	cmdplacement "github.com/openshift-online/gecko/controllers/cmd/placement"
	cmdversionresolution "github.com/openshift-online/gecko/controllers/cmd/versionresolution"
	"github.com/openshift-online/gecko/controllers/util/logger"
)

// RootFlags holds persistent flags shared by all subcommands.
// Exported so cmd/ packages can reference it.
type RootFlags struct {
	LogLevel  string
	LogFormat string
	OrlopURL  string
	Workers   int
}

// NewManager creates a controller-runtime Manager and scheme with platform-api types registered.
func (rf *RootFlags) NewManager(log logger.Logger) (*runtime.Scheme, ctrl.Manager, error) {
	scheme := runtime.NewScheme()
	if err := privatev1.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("register types: %w", err)
	}
	ctrl.SetLogger(logger.ToLogr(log))
	mgr, err := ctrl.NewManager(&rest.Config{Host: rf.OrlopURL}, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create manager: %w", err)
	}
	return scheme, mgr, nil
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func main() {
	rf := &RootFlags{}

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
```

> **Note:** `RootFlags` is defined in `main.go` (package `main`). The `cmd/` subcommand packages import the root package — this creates an import cycle. To break it, extract `RootFlags` and `NewManager` into a small shared package, e.g. `rootflags/rootflags.go`, and have both `main.go` and `cmd/` packages import from there. Update Step 1–5 to import `rootflags` instead of having `rf *RootFlags` reference `main` package.

- [ ] **Step 7: Extract RootFlags to avoid import cycle**

Create `controllers/rootflags/rootflags.go`:

```go
package rootflags

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	privatev1 "github.com/openshift-online/gecko/platform-api/api/private/v1"
	"github.com/openshift-online/gecko/controllers/util/logger"
)

// RootFlags holds persistent flags shared across all subcommands.
type RootFlags struct {
	LogLevel  string
	LogFormat string
	OrlopURL  string
	Workers   int
}

// NewManager creates a runtime.Scheme with platform-api types and a controller-runtime Manager.
func (rf *RootFlags) NewManager(log logger.Logger) (*runtime.Scheme, ctrl.Manager, error) {
	scheme := runtime.NewScheme()
	if err := privatev1.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("register types: %w", err)
	}
	ctrl.SetLogger(logger.ToLogr(log))
	mgr, err := ctrl.NewManager(&rest.Config{Host: rf.OrlopURL}, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create manager: %w", err)
	}
	return scheme, mgr, nil
}
```

Update `main.go` to remove the `RootFlags`/`NewManager` definitions and import `rootflags` instead.
Update all `cmd/*/cmd.go` files to import `rootflags` and use `rf *rootflags.RootFlags`.

- [ ] **Step 8: Full build**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go build ./...
```

Expected: no errors, binary compiles

- [ ] **Step 9: Smoke test the binary**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go run . --help
```

Expected output includes `version-resolution`, `nodepool-vr`, `placement`, `hc`, `nodepool` subcommands listed.

- [ ] **Step 10: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/main.go controllers/rootflags/ controllers/cmd/
git commit -m "feat(controllers): wire up main.go and subcommands"
```

---

## Task 12: Create Makefile and Dockerfile

**Files:**
- Create: `controllers/Makefile`
- Create: `controllers/Dockerfile`

- [ ] **Step 1: Create controllers/Makefile**

```makefile
.PHONY: build test lint docker-build

BINARY_NAME=gecko-controllers
IMAGE=quay.io/gcphcp/gecko-controllers

build:
	go build -o bin/$(BINARY_NAME) .

test:
	go test ./...

lint:
	golangci-lint run ./...

docker-build:
	docker build -t $(IMAGE):latest .
```

- [ ] **Step 2: Create controllers/Dockerfile**

```dockerfile
FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder
USER root
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/gecko-controllers .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/gecko-controllers /gecko-controllers
ENTRYPOINT ["/gecko-controllers"]
```

> Note: The Dockerfile copies only the `controllers/` directory. The `replace` directive for `platform-api` points to `../platform-api`, which won't be available inside the Docker build context. Two options:
> 1. Copy `platform-api/` into the build context alongside `controllers/` (update `context:` in CI)
> 2. Publish `platform-api` as a proper Go module and remove the `replace` directive
>
> For now, document this in a comment in the Dockerfile — resolving it is a follow-up.

- [ ] **Step 3: Commit**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/Makefile controllers/Dockerfile
git commit -m "feat(controllers): add Makefile and Dockerfile"
```

---

## Task 13: Full Build and Test Verification

- [ ] **Step 1: Run the full test suite**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go test ./... -v 2>&1 | tail -30
```

Expected: all tests PASS. If any test fails, read the error — it will point to a missed import substitution or a structural difference between old and new types. Fix the specific file and re-run.

- [ ] **Step 2: Run go mod tidy to clean up**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go mod tidy
```

Expected: no changes, or minor cleanup of indirect dependencies.

- [ ] **Step 3: Verify the binary builds cleanly from scratch**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko/controllers
go clean -cache && go build ./...
```

Expected: succeeds

- [ ] **Step 4: Final commit if go.mod/go.sum changed after tidy**

```bash
cd /home/cveiga/go/src/github.com/openshift-online/gecko
git add controllers/go.mod controllers/go.sum
git diff --staged --quiet || git commit -m "feat(controllers): finalize go.mod after tidy"
```