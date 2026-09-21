# netbird-terraformer

Imports an existing [NetBird](https://netbird.io) installation into Terraform
configuration: it reads your account through the NetBird REST API and writes
`.tf` files plus the `import` blocks that bind them to the live resources.

It talks to the API over plain HTTP with the Go standard library, so it carries
no gRPC dependency and does not conflict with the main
[Terraformer](https://github.com/GoogleCloudPlatform/terraformer) project.

## Features

- **Complete resource coverage** — groups, users, policies, routes, networks,
  DNS, posture checks and reverse proxies, with their full attribute set
- **Reference resolution** — resource IDs become Terraform references
  (`netbird_group.foo.id`) instead of opaque strings, so a rename does not break
  the graph
- **Deterministic output** — attributes are emitted in a stable order, so
  re-running the tool produces a reviewable diff rather than a reshuffled file
- **Import blocks and state reconciliation** — generates `imports.tf`, with
  helper scripts to prune it against existing state and to reconcile state
  without touching live infrastructure
- **Zero external dependencies** — Go standard library only

## Installation

```bash
# Build from source
go build -o netbird-terraformer .

# Or via make
make build
```

### With Nix (no checkout needed)

The flake is the intended way to consume this from another repository: pin a tag
and you get a reproducible tool without vendoring the source.

```bash
nix run github:UberM1/netbird_terraformer/v0.1.1 -- --help
nix run github:UberM1/netbird_terraformer/v0.1.1#reconcile-state -- --dry-run
nix run github:UberM1/netbird_terraformer/v0.1.1#prune-imports -- imports.tf state.txt
```

| Flake output | What it is |
|---|---|
| `.` / `.#netbird-terraformer` | the importer |
| `.#prune-imports` | `prune_imports.sh` |
| `.#reconcile-state` | `reconcile_state.sh` |
| `.#helpers` | both scripts, plus `share/netbird-terraformer/backend-env.sh` |

The helpers shell out to `terraform`, which stays yours to provide: pinning a
version in the flake would override whatever the consuming project uses.

A dev shell with Go, gopls and OpenTofu is available via `nix develop`.

## Configuration

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `NB_PAT` | yes | — | NetBird personal access token |
| `NB_MANAGEMENT_URL` | no | `https://api.netbird.io` | Management API URL; set it for self-hosted installs, including the port |
| `DEBUG` | no | `false` | Log every API request |
| `AUTO_IMPORT` | no | `true` | Run `terraform import` after generating; set to `false` to only write files |

```bash
export NB_PAT="pat_your_token_here"
export NB_MANAGEMENT_URL="https://netbird.example.com:33073"   # self-hosted only
```

## Usage

```bash
./netbird-terraformer                      # writes to ./generated
./netbird-terraformer my-terraform-config  # writes to a directory of your choice
./netbird-terraformer --help
./netbird-terraformer --debug-auth         # test the token against /api/groups
```

## Generated files

```
generated/
├── provider.tf           # provider + required_providers
├── variables.tf          # netbird_token, netbird_management_url
├── group.tf              # and one file per resource type below
├── ...
├── clusters.tf           # reverse proxy cluster lookup map
├── imports.tf            # terraform import blocks
├── group_mappings.json   # group id -> terraform address, for reference
└── import.sh             # legacy, superseded by imports.tf
```

## Resource coverage

| API endpoint | Generated |
|---|---|
| `/api/groups` | `netbird_group` |
| `/api/users` | `netbird_user` |
| `/api/policies` | `netbird_policy` |
| `/api/routes` | `netbird_route` |
| `/api/networks` | `netbird_network`, `netbird_network_resource`, `netbird_network_router` |
| `/api/posture-checks` | `netbird_posture_check` |
| `/api/dns/zones` | `netbird_dns_zone`, `netbird_dns_record` |
| `/api/dns/nameservers` | `netbird_nameserver_group` |
| `/api/dns/settings` | `netbird_dns_settings` (no import — singleton) |
| `/api/reverse-proxies/domains` | `netbird_reverse_proxy_domain` |
| `/api/reverse-proxies/services` | `netbird_reverse_proxy_service` |
| `/api/peers` | `data.netbird_peer` (peers are managed by the client, not Terraform) |

Generation order matters and is handled for you: peers resolve first (routes and
network routers reference them), then groups (almost everything references
them), then networks (policies and reverse proxies reference them).

## Post-import workflow

With `AUTO_IMPORT=true` (the default) the resources are already in state:

```bash
cd generated
terraform plan      # a clean import leaves no changes
```

With `AUTO_IMPORT=false`, bind them through the generated import blocks:

```bash
cd generated
terraform state list > state.txt
nix run github:UberM1/netbird_terraformer#prune-imports -- imports.tf state.txt
terraform plan                                        # review
terraform apply                                       # binds, then delete imports.tf
```

## Helper scripts

**`prune_imports.sh`** — Terraform fails the whole plan if an `import` block
targets a resource it already manages. This drops the blocks that are already in
state.

```bash
terraform state list > state.txt
bash prune_imports.sh imports.tf state.txt
# or, without a checkout:
nix run github:UberM1/netbird_terraformer#prune-imports -- imports.tf state.txt
```

**`reconcile_state.sh`** — closes the gap between state and configuration
*without* touching NetBird. `terraform apply` on import blocks would also run
every create/update/destroy in the plan; `terraform import` and
`terraform state mv` are state-only. The script reads `moved.tf` for renames and
`imports.tf` for imports, and is idempotent, so it is safe to re-run after an
interrupted run.

```bash
bash reconcile_state.sh --dry-run
bash reconcile_state.sh
# or, without a checkout:
nix run github:UberM1/netbird_terraformer#reconcile-state -- --dry-run
```

**`backend-env.sh`** — optional, for a GitLab-managed remote state
(backend `"http"`). Holds no secrets; it maps tokens from your shell onto the
`TF_HTTP_*` variables Terraform expects.

```bash
export GITLAB_TOKEN=<gitlab PAT, api scope>
export NB_PAT=<netbird PAT>
export TF_STATE_PROJECT_ID=<numeric GitLab project id>
export TF_STATE_NAME=netbird
source /path/to/backend-env.sh
```

It also forces direct registry installation of the provider: a stale
`netbirdio/netbird` copy under `~/.terraform.d/plugins` is an implied filesystem
mirror that shadows the registry, and `terraform validate` then reports bogus
"resource type not supported" errors. Set `TF_SKIP_DIRECT_INSTALL=true` to keep
your own CLI config.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `NB_PAT environment variable is required` | Token not exported |
| `API request failed with status 401` | Invalid token, or missing permissions |
| `API request failed with status 404` | Wrong management URL — self-hosted installs usually need an explicit port |
| Empty resources in output | The token lacks read permission for that resource type |
| HTML instead of JSON | The URL points at the dashboard, not the API |
| `resource type not supported` on a valid resource | A stale provider in the local filesystem mirror — see `backend-env.sh` above |

```bash
# Test API access by hand
curl -H "Authorization: Token $NB_PAT" "$NB_MANAGEMENT_URL/api/groups"

# Or through the tool
DEBUG=true ./netbird-terraformer --debug-auth
```

## Extending

The architecture is one generator per resource type:

- **New resource type** — implement `lib.ResourceHandler` in `resources/`, then
  register it in `main.go`
- **Terraform output** — `lib/terraform.go`
- **Configuration** — `config.go`
- **API transport** — `service.go`

Tests live in `test/` and cover the deterministic attribute ordering that keeps
regenerated output reviewable:

```bash
go test ./...
```

## License

Apache License 2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
