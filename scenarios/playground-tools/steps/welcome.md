# Tools Playground

This sandbox has everything pre-installed. No setup required — start experimenting immediately.

## Available tools

| Tool | Command |
|------|---------|
| Docker | `docker` |
| k3d | `k3d` |
| uds CLI | `uds` |
| kubectl | `uds zarf tools kubectl` or `kubectl` |
| Neovim | `nvim` |
| jq | `jq` |
| yq | `yq` |
| yamllint | `yamllint` |

## Getting started

Create a k3d cluster:

```
k3d cluster create lab
```

Deploy UDS Core:

```
uds deploy k3d-core-slim-dev:latest --confirm
```

The browser preview shows a Chromium window inside the VM — useful for accessing UDS services at `https://sso.uds.dev`.
