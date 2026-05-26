# ADR-0001: Bundled Chromium via Browser Mode for *.uds.dev Access

**Status:** Accepted

## Context

UDS Core services are exposed via Istio VirtualServices on the `*.uds.dev` domain. The `uds.dev` domain resolves to `127.0.0.1` — meaning these services are only reachable from the loopback interface of the machine running the cluster. Lab VMs run on remote Hetzner hosts; users access them through a browser on a different machine entirely.

Alternatives considered:
- **Port-forwarding**: Requires `kubectl port-forward` per service, doesn't scale to multi-service demos, breaks the "no local install" goal.
- **External ingress with real DNS**: Requires public DNS, TLS certificates, and exposes services to the internet — inappropriate for ephemeral lab VMs.
- **Custom browser extension / proxy**: Complex, requires client-side installation, defeats the browser-only access model.

## Decision

Bundle Chromium in the Base Image and provide Browser Mode (`browser: true` in scenario.yaml). When enabled, the VM starts Xvfb + x11vnc + noVNC, and the Lab UI exposes a button that streams the VM desktop to the user's browser via WebSocket. The VM's Chromium runs locally on the VM (loopback), so `*.uds.dev` resolves correctly.

## Consequences

- Users access UDS services without any local tooling or port-forwarding.
- The browser is always Chromium — no user choice, but also no configuration complexity.
- Browser Mode adds noVNC streaming overhead; only enabled for scenarios that need it.
- All VM images carry Chromium even when Browser Mode is not used (baked into Base Image).
