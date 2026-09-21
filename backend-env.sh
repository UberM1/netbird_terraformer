#!/usr/bin/env bash
# Sourceable environment for running Terraform against a GitLab-managed
# remote state (backend "http") with the NetBird provider.
#
# Contains NO secrets: it reads the tokens from your shell and maps them onto
# the variables Terraform expects.
#
#   export GITLAB_TOKEN=<gitlab PAT, api scope>
#   export NB_PAT=<netbird personal access token>
#   export TF_STATE_PROJECT_ID=<numeric GitLab project id holding the state>
#   export TF_STATE_NAME=<state name>            # default: netbird
#   export TF_STATE_HOST=<gitlab host>           # default: https://gitlab.com
#   cd <your terraform project directory>
#   source /path/to/backend-env.sh

if [ -z "${GITLAB_TOKEN:-}" ]; then
  echo "ERROR: export GITLAB_TOKEN first (GitLab PAT with api scope)" >&2
  return 1 2>/dev/null || exit 1
fi
if [ -z "${NB_PAT:-}" ]; then
  echo "ERROR: export NB_PAT first (NetBird personal access token)" >&2
  return 1 2>/dev/null || exit 1
fi
if [ -z "${TF_STATE_PROJECT_ID:-}" ]; then
  echo "ERROR: export TF_STATE_PROJECT_ID first (numeric GitLab project id)" >&2
  return 1 2>/dev/null || exit 1
fi

_STATE_HOST="${TF_STATE_HOST:-https://gitlab.com}"
_STATE_NAME="${TF_STATE_NAME:-netbird}"

# --- GitLab-managed remote state (backend "http") ---------------------------
export TF_HTTP_ADDRESS="${_STATE_HOST}/api/v4/projects/${TF_STATE_PROJECT_ID}/terraform/state/${_STATE_NAME}"
export TF_HTTP_LOCK_ADDRESS="${TF_HTTP_ADDRESS}/lock"
export TF_HTTP_UNLOCK_ADDRESS="${TF_HTTP_ADDRESS}/lock"
export TF_HTTP_LOCK_METHOD="POST"
export TF_HTTP_UNLOCK_METHOD="DELETE"
export TF_HTTP_RETRY_WAIT_MIN="5"
export TF_HTTP_USERNAME="${GITLAB_USERNAME:-$USER}"
export TF_HTTP_PASSWORD="${GITLAB_TOKEN}"

# --- NetBird provider -------------------------------------------------------
export TF_VAR_netbird_token="${NB_PAT}"

# --- Provider installation --------------------------------------------------
# ~/.terraform.d/plugins is an implied filesystem mirror and takes precedence
# over the registry, so a stale netbirdio/netbird copy there shadows the version
# the configuration asks for and `terraform validate` reports bogus
# "resource type not supported" errors. Force direct registry installation for
# this shell only. Set TF_SKIP_DIRECT_INSTALL=true to keep your own CLI config.
if [ "${TF_SKIP_DIRECT_INSTALL:-false}" != "true" ]; then
  _TFRC="${TMPDIR:-/tmp}/netbird-terraformer.tfrc"
  printf 'provider_installation {\n  direct {}\n}\n' > "$_TFRC"
  export TF_CLI_CONFIG_FILE="$_TFRC"
  unset _TFRC
fi

echo "Terraform environment ready."
echo "  state    : ${TF_HTTP_ADDRESS}"
echo "  provider : ${TF_CLI_CONFIG_FILE:+direct registry install}${TF_CLI_CONFIG_FILE:-using your existing CLI config}"
unset _STATE_HOST _STATE_NAME
