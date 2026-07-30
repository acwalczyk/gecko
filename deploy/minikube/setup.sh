#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Use podman or docker, whichever is available.
if command -v podman &>/dev/null; then
  BUILDER=podman
elif command -v docker &>/dev/null; then
  BUILDER=docker
else
  echo "Error: neither podman nor docker found" >&2
  exit 1
fi

build_and_load() {
  local name="$1" containerfile="$2"
  local image="localhost/${name}:latest"
  echo "==> Building $name..."
  $BUILDER build -f "$containerfile" -t "$image" "$REPO_ROOT"
  echo "==> Loading $name into minikube..."
  local tmptar
  tmptar="$(mktemp --suffix=.tar)"
  $BUILDER save -o "$tmptar" "$image"
  minikube image load "$tmptar"
  rm -f "$tmptar"
}

echo "==> Checking minikube status..."
if ! minikube status &>/dev/null; then
  echo "==> Starting minikube..."
  minikube start
fi

# Ensure kubectl is targeting minikube before touching any cluster.
CURRENT_CTX="$(kubectl config current-context 2>/dev/null || true)"
if [[ "$CURRENT_CTX" != "minikube" ]]; then
  echo "Error: current kubectl context is '${CURRENT_CTX:-<none>}', expected 'minikube'" >&2
  echo "Run: kubectl config use-context minikube" >&2
  exit 1
fi

echo "==> Installing cert-manager..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
echo "==> Waiting for cert-manager webhook..."
kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=120s

echo "==> Creating self-signed ClusterIssuer..."
kubectl apply -f "$SCRIPT_DIR/clusterissuer.yaml"

build_and_load platform-api-server "$SCRIPT_DIR/../platform-api/Containerfile"

echo "==> Deploying..."
kubectl apply -k "$SCRIPT_DIR"

echo "==> Restarting deployment to pick up new image..."
kubectl -n gecko-system rollout restart deploy/platform-api-server

echo "==> Waiting for platform-api-server..."
kubectl -n gecko-system rollout status deploy/platform-api-server --timeout=120s

echo "==> Verifying API registration..."
kubectl get apiservice v1.gcp.managed.openshift.io

echo ""
echo "Done."
