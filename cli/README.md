<!--
Source: getarcaneapp/arcane/cli/README.md
Revision: 095cb778daba1034e2a39961199e712a75d4ba29
Retrieved: 2026-10-13
License: BSD-3-Clause
Deviations: Local module namespace; functional cut: local-account login replaces OIDC device-code login; removed upstream branding, missing logo and unsupported module-tag guidance; command examples use the installed binary name.
-->

# Command line client

## Install

The CLI module lives in this repository. To install it:

- `go install github.com/florianfinn/docklet-next/cli@latest`

## Configure

The CLI stores config in `~/.config/arcanecli.yml`.

Create a starter config file (all supported keys):

- `cli config init`
- `cli config backup` (moves current config to `~/.config/arcanecli.yml.bak`)

Set the hub URL:

- `cli config set server-url http://localhost:3552`

### Authenticate (choose one)

Both options require the hub's `local` sign-in mode. The CLI supplies explicit
credentials without browser cookies, Origin or CSRF headers. Local login and
refresh accept cookie-less requests; authenticated writes validate the Bearer
token or API key. Proxy mode requires authentication through the configured proxy.

#### Option A: Local account

- `cli auth login --username <name>` (prompts for the password)

#### Option B: API key

- `cli config set api-key arc_xxxxxxxxxxxxx`

## Useful Global Flags

- `--output text|json` for output mode (`--json` is an alias for `--output json`)
- `--env <id>` to override the configured default environment for one command
- `--yes` to auto-confirm destructive prompts
- `--no-color` to disable ANSI color output
- `--request-timeout <duration>` to override HTTP timeout per command

## Utilities

- `cli completion bash|zsh|fish|powershell` to generate shell completions
- `cli doctor` to run local CLI diagnostics

## Pagination Config

Set global and per-resource list limits in config:

```yaml
pagination:
  default:
    limit: 25
  resources:
    containers:
      limit: 50
    images:
      limit: 100
    volumes:
      limit: 40
    networks:
      limit: 40
```

CLI precedence is:
1. `--limit`
2. `pagination.resources.<resource>.limit`
3. `pagination.default.limit`
4. command built-in default

You can configure limits with:
- `cli config set default-limit 25`
- `cli config set pagination.resources.containers.limit 50 pagination.resources.images.limit 100`

Legacy flag syntax remains supported:
- `cli config set --default-limit 25`
- `cli config set --resource-limit containers=50 --resource-limit images=100`
