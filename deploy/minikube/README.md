# platform-api on minikube

## Setup

```sh
./deploy/minikube/setup.sh
```

Builds `localhost/platform-api-server:latest`, loads it into minikube, applies the
`deploy/minikube` kustomize overlay.

Overlay-specific changes vs. `deploy/platform-api/base`:
- `--enable-public-api=true` (base defaults to `false`)
- `imagePullPolicy: Never` (use the locally built image)
- extra `platform-api-server-public` Service (NodePort 8081:30081), minikube-only

## Reaching the public endpoint

```sh
kubectl -n gecko-system port-forward svc/platform-api-server-public 8081:8081
kubectl --server http://localhost:8081 get cluster -o yaml
```

Or with curl:

```sh
curl http://localhost:8081/apis/gcp.managed.openshift.io/v1/namespaces/default/clusters
```

### Driver caveat

On the `qemu2` driver with builtin networking, `minikube ip` returns a SLIRP
address (e.g. `10.0.2.15`) that isn't routable from the host, and
`minikube service` fails with `MK_UNIMPLEMENTED: minikube service is not
currently implemented with the builtin network on QEMU`. The NodePort service
still works on other drivers (docker, socket_vmnet network, CI) via
`minikube service platform-api-server-public -n gecko-system --url` or
`curl http://$(minikube ip):30081/...`. On qemu2/builtin, use `kubectl
port-forward` instead — it's driver-agnostic.

## Teardown

```sh
./deploy/minikube/teardown.sh
```
