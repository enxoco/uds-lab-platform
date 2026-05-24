# uds-lab-platform

Interactive lab platform for UDS training. Coder-based, Hetzner-backed, managed with Atmos + OpenTofu.

## How it works

- **Coder** manages user workspaces via browser-based terminal
- **Hetzner** provisions ephemeral VMs per lab session (~$0.027/hr for cx41)
- **Atmos** manages the Coder server infrastructure
- Lab workspaces boot with either `uds deploy k3d-core-slim-dev:latest` or `zarf init` on k3d

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [OpenTofu](https://opentofu.org/docs/intro/install/)
- [Atmos](https://atmos.tools/install)
- [Coder CLI](https://coder.com/docs/install)
- Hetzner Cloud account + API token

## Local development

```bash
cp .env.example .env
docker compose up -d
```

Coder available at http://localhost:3000. Create admin account on first visit.

## Deploy Coder server to Hetzner

```bash
export HCLOUD_TOKEN=your_token_here
export TF_VAR_postgres_password=your_password_here

atmos terraform apply coder-server --stack dev
```

After apply, set `CODER_ACCESS_URL` in `.env` (or stack vars) to the output IP:

```
CODER_ACCESS_URL=http://<server_ip>:3000
```

## Push workspace template

Authenticate Coder CLI against your server:

```bash
coder login http://localhost:3000   # or remote URL
```

Push the template:

```bash
coder templates push uds-workspace --directory templates/uds-workspace/
```

Set the Hetzner token as a template variable in Coder UI, or ensure `HCLOUD_TOKEN` is set in the Coder server's environment.

## Creating a workspace

In the Coder UI, create a workspace from the `uds-workspace` template. Choose setup mode:

| Mode | Command run | Use when |
|------|-------------|----------|
| UDS Core Slim Dev | `uds deploy k3d-core-slim-dev:latest` | Full UDS stack (Istio, Keycloak) |
| Zarf Init | `k3d cluster create` + `zarf init` | Zarf-only scenarios |

## Project structure

```
atmos.yaml                        Atmos config (uses OpenTofu)
docker-compose.yml                Local Coder + Postgres
renovate.json                     Dependency updates (3-day min age)
stacks/
  catalog/coder-server.yaml       Default server vars
  dev/dev.yaml                    Dev environment stack
components/terraform/
  coder-server/                   Hetzner VM + firewall for Coder
templates/
  uds-workspace/                  Coder workspace template
    main.tf                       Hetzner VM per session, coder_agent
    variables.tf
    scripts/
      startup.sh                  Installs tooling, bootstraps cluster
      user-data.sh.tftpl          Bootstraps Coder agent on VM boot
```

## Costs

| Resource | Type | Cost |
|----------|------|------|
| Coder server | ccx13 (8GB dedicated) | ~$0.020/hr |
| Lab workspace | cx41 (16GB) | ~$0.027/hr |
| Idle workspace (stopped) | — | $0 |

Workspaces stopped when idle; VMs destroyed on workspace delete.
