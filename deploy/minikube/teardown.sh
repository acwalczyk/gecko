#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> Removing platform-api-server..."
kubectl delete -k "$SCRIPT_DIR" --ignore-not-found

echo "==> Removing ClusterIssuer..."
kubectl delete -f "$SCRIPT_DIR/clusterissuer.yaml" --ignore-not-found

echo "Done."
