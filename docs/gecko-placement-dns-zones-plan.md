# Gecko Placement — HC DNS Zones via Helm Injection

## Context

Hosted cluster (HC) DNS zone domains for a region are provisioned by Terraform
(`terraform/modules/region/`) and currently exposed to the cluster only via the
`meta_hc_dns_domains` field on the `argocd-cluster` Secret Manager secret
(see `terraform/modules/region/secrets.tf`,
`google_secret_manager_secret_version.argocd_cluster`).

Today, `gecko-placement`'s `DynamicSelector` reads that secret directly
(`controllers/placement/dynamic_selector.go`, `hcDNSDomains()`) to discover the
list of available domains for round-robin DNS zone selection during
placement.

This is being replaced: `gcp-hcp-infra` now surfaces the same value to ArgoCD
as `{{ .Values.hc_dns_domains }}` (already used for the ApplicationSet cluster
metadata), and injects it into the `gecko-placement` Helm chart as
`hcDnsDomains`
(`argocd/config/region/gecko-placement/template.yaml`, PR #1174).

**gecko-placement should no longer read the `argocd-cluster` secret at all.**
DNS domains are received as a Helm value → CLI flag, same mechanism already
used for `secretManagerProject`.

This also aligns with the companion `mc-registration-plan.md`: MC discovery is
moving to `mc-registration-*` secrets, and the `gecko-placement` GSA's IAM
bindings are being conditioned to only those secrets
(`helm/charts/gecko-cloud-resources/templates/iam.yaml`). Once that IAM
condition lands, `gecko-placement` loses read access to `argocd-cluster`
anyway — this change removes the now-unusable code path.

## Why DNS domains are not part of the mc-registration secret

DNS zones are a **region-level** resource, not a per-MC one — a region can
have multiple HC DNS zones (`hc_zone_count`), shared across all MCs in that
region. They are not a 1:1 mapping with management clusters, so they don't
belong in the per-MC `mc-registration-{project-id}` secret payload. Terraform
→ ArgoCD → Helm injection keeps DNS zone data flowing through the same
mechanism as other region-level cluster metadata (`project_id`, `region`,
etc.).

## `--base-domains` / `--candidates` are being removed, not reused

The existing static flags don't fit the target model and are removed
entirely rather than adapted:

- `--candidates` (static MC name list) is obsoleted by SM-based MC discovery
  (`mc-registration-plan.md`) — MC eligibility depends on `mode: active`,
  which is only knowable by reading Secret Manager. There is no static
  fallback that can express this.
- `--base-domains` is index-paired 1:1 with `--candidates` (one domain per
  MC). That model is wrong: DNS domains are a shared region-level pool, not
  assigned per-MC.

Net effect: `DynamicSelector` (Secret Manager driven) becomes the only
selector implementation. The static `RoundRobinSelector` / `Candidate`-list
path is deleted.

## Proposed changes

### Helm chart (`helm/charts/gecko-placement/`)

**`values.yaml`**
- Add `hcDnsDomains: ""` — comma-separated string, injected by ArgoCD from
  Terraform via `{{ .Values.hc_dns_domains }}`.
- Remove `candidates: []`.
- Remove `baseDomains: []`.

**`templates/deployment.yaml`**
- Add, unconditionally alongside `--secretmanager-project`:
  ```yaml
  {{- if .Values.hcDnsDomains }}
  - --hc-dns-domains={{ .Values.hcDnsDomains }}
  {{- end }}
  ```
- Remove the `{{- else }}` branch that renders `--candidates` /
  `--base-domains`.

**`templates/_helpers.tpl`**
- Update `gecko-placement.validateValues`: require both
  `secretManagerProject` and `hcDnsDomains` to be non-empty. Drop the
  `candidates`-as-fallback validation branch.

### Controller

**`controllers/cmd/placement/cmd.go`**
- Remove `candidateNames`, `baseDomains` vars, the `--candidates` /
  `--base-domains` flags, and the index-pairing loop that builds
  `[]Candidate`.
- Add `--hc-dns-domains` string flag (comma-separated). Split into
  `[]string`, trimming whitespace and dropping empty entries (same parsing
  gecko already does internally for `meta_hc_dns_domains`).
- Pass the parsed domain list into the `DynamicSelector` constructor.

**`controllers/placement/dynamic_selector.go`**
- Remove `hcDNSDomains()` entirely — no more SM reads of the
  `argocd-cluster` secret.
- Constructor accepts `domains []string` directly.
- Keep the existing round-robin selection logic
  (`domains[s.domCounter.Add(1)%uint64(len(domains))]`), sourced from the
  injected slice instead of a live SM lookup.
- If `domains` is empty at construction, fail fast (same behavior as today's
  "no HC DNS domains found" error, just checked once instead of per-call).

**`controllers/placement/selector.go`**
- Remove the `Candidate` struct, the `Selector` interface (single
  implementation remains, interface no longer earns its keep), and
  `RoundRobinSelector`.

**`controllers/placement/placement_controller.go`**
- Drop the `r.candidates` field / static selector wiring; construct
  `DynamicSelector` directly with `secretManagerProject` +
  `hcDnsDomains`.

**Tests**
- Remove static-candidate / static-base-domain test cases in
  `placement_controller_test.go`.
- Add coverage for `--hc-dns-domains` parsing and `DynamicSelector` behavior
  with an injected domain list (including the "empty list" failure case).

## Data flow (target)

```
MC discovery:  mc-registration-* SM secrets  → DynamicSelector → active MCs
DNS domains:   Terraform → ArgoCD values (hc_dns_domains)
                 → gecko-placement Helm value (hcDnsDomains)
                   → --hc-dns-domains flag → DynamicSelector
Selection:     round-robin(active MCs) × round-robin(dns domains)
                 → cluster.Status.PlacementResult{ManagementClusterName, BaseDomain}
```

## IAM

No new IAM changes required here. `helm/charts/gecko-cloud-resources` (in
`gcp-hcp-infra`) already conditions the `gecko-placement` GSA's
`secretmanager.viewer` / `secretmanager.secretAccessor` bindings to
`mc-registration-*` secrets only (see `mc-registration-plan.md`). Removing
the `argocd-cluster` read in this plan is what makes that IAM restriction
safe to apply without breaking DNS zone discovery.

## Migration path

1. `gcp-hcp-infra` PR #1174 merges: `hcDnsDomains` value flows through
   ArgoCD to the `gecko-placement` Application (already implemented).
2. This plan's chart + controller changes land in `gecko`: controller reads
   `--hc-dns-domains` instead of the `argocd-cluster` secret; `--candidates`
   / `--base-domains` removed.
3. Deploy together (or controller change first with the flag simply unused
   until infra catches up — `DynamicSelector` requires `secretManagerProject`
   already, so no behavior change until `hcDnsDomains` is also wired).
4. Once `mc-registration-plan.md`'s IAM conditioning is applied, confirm
   `gecko-placement` no longer needs (and doesn't have) access to
   `argocd-cluster`.
