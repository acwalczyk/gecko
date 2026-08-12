# MC Registration — Gecko Placement Changes

## Context

`gcp-hcp-infra` creates per-MC secrets in the region project's Secret Manager:

- **Name**: `mc-registration-{mc-project-id}` (e.g., `mc-registration-int-mgt-us-c1-a1b2`)
- **Labels**: `mc-registration=true`, `cluster-type=management-cluster`
- **Content**: JSON `{"projectId": "...", "mode": "active|maintenance"}`
- **Source**: `terraform/modules/management-cluster/region-registration.tf`

The gecko-placement controller reads these secrets directly from Secret Manager
to discover eligible management clusters and their operational status.

## IAM

Managed by `gcp-hcp-infra` via `helm/charts/gecko-cloud-resources/templates/iam.yaml`.

The `gecko-placement` GSA has two conditioned IAM bindings on the region project:

| Role | Purpose | Condition |
|------|---------|-----------|
| `roles/secretmanager.viewer` | List/discover mc-registration-* secrets | `resource.name.contains("/secrets/mc-registration-")` |
| `roles/secretmanager.secretAccessor` | Read mc-registration-* secret values | `resource.name.contains("/secrets/mc-registration-")` |

Both restricted via IAM condition to mc-registration secrets only.

## Controller Changes

### File: `controllers/placement/dynamic_selector.go`

#### 1. Replace SM discovery logic

**Current**: `smMCNames()` lists secrets with `maestro-consumer-name:*` label,
intersects with Maestro HTTP API consumer list.

**New**: List secrets with label `mc-registration=true`. For each secret, read
the JSON value via `AccessSecretVersion`. Build candidate list from secrets
where `mode == "active"`.

- Remove Maestro consumer API dependency (obsoleted by mc-registration secrets)
- Remove `maestro-consumer-name` label reading
- Read secret values (JSON) instead of just labels
- Filter: only MCs with `mode == "active"` are eligible for new placements

#### 2. Candidate struct

Extend `Candidate` (or equivalent) with fields from secret JSON:

- `projectId` — MC project ID (string)
- `mode` — operational mode: `active` or `maintenance` (string)

The struct is extensible: new fields added to the Terraform JSON payload
become available automatically without ExternalSecret or intermediary changes.

#### 3. Mode filtering

- `active` — MC eligible for new hosted cluster placements
- `maintenance` — MC excluded from new placements (existing hosted clusters stay)

#### 4. DNS domain discovery

**Current**: `hcDNSDomains()` reads `argocd-cluster` secret for `meta_hc_dns_domains`.
This is NOT an mc-registration secret.

**Options** (decide which):
- Add `baseDomain` field to mc-registration secret JSON (preferred — single source of truth per MC)
- Keep separate DNS discovery mechanism

### File: `controllers/cmd/placement/cmd.go`

#### 5. Flag changes

- `--secretmanager-project` flag stays (same purpose, new discovery logic)
- Remove `--candidates` / `--base-domains` static flags (or keep for local testing only)
- Consider: `--mc-registration-label` flag for configurability
  (default: `mc-registration=true`)

## Helm Chart Changes

### File: `helm/charts/gecko-placement/values.yaml`

#### 1. Remove static candidate values

Remove or deprecate `candidates` and `baseDomains` lists.
`secretManagerProject` becomes the only discovery mechanism.

#### 2. Add mc-registration config (optional)

```yaml
mcRegistration:
  label: "mc-registration"  # SM label key for discovery
```

### File: `helm/charts/gecko-placement/templates/_helpers.tpl`

#### 3. Update validation

Remove validation requiring `secretManagerProject OR candidates`.
`secretManagerProject` becomes required (no static fallback).

### File: `helm/charts/gecko-placement/templates/deployment.yaml`

#### 4. Update container args

Remove `--candidates` / `--base-domains` arg injection.
Keep `--secretmanager-project` arg.

## Migration Path

1. **gcp-hcp-infra PR** (already implemented):
   - Terraform creates `mc-registration-*` secrets alongside existing `mc-remote-gke-cluster`
   - IAM updated with conditioned viewer + accessor

2. **gecko PR** (this plan):
   - Update DynamicSelector to read mc-registration secrets
   - Remove Maestro consumer dependency
   - Update Helm chart (remove static candidates)

3. **Cleanup PR** (follow-up):
   - Remove old `maestro-consumer-name` label from `cls-registration.tf`
   - Remove unconditioned `secretmanager.viewer` if still present
   - Deprecate `mc-remote-gke-cluster` secret once CLS no longer needs it
