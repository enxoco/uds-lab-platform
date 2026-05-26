# ADR-0003: Reference Package Scenario Uses UDS Core Playground Snapshot

**Status:** Accepted

## Context

The `uds-reference-package` scenario teaches UDS packaging by exploring and deploying the reference package against a running UDS Core cluster. The scenario has two options for how that cluster is provisioned:

- **Deploy UDS Core as a scenario step** — user runs `uds deploy k3d-core-slim-dev:latest`, waits 5–10 minutes, then begins learning about packaging. The deployment itself is not the learning objective.
- **Start from the `playground-uds-core` snapshot** — UDS Core is pre-deployed in the VM image; user lands in a live cluster immediately.

The platform's image selection logic previously only applied pre-built playground snapshots to scenarios with `playground: true`. Non-playground (guided) scenarios always received the base image. This created a gap: a guided scenario needing a pre-deployed cluster had no mechanism to request one without being reclassified as a playground.

## Decision

Add an `image` field to `scenario.yaml`. When set, the session manager uses `role=uds-lab-playground,tier=<image>` as the snapshot label selector, regardless of the `playground` flag. The `uds-reference-package` scenario sets `image: uds-core`.

The session manager change is in `internal/session/manager.go`.

## Consequences

- The reference package scenario starts with UDS Core live — no setup wait, no distraction from the packaging learning objective.
- The `image` field is available to any future guided scenario that needs a pre-built environment without being an open-ended playground.
- Scenarios using `image` depend on the named snapshot existing in Hetzner. If the snapshot is missing, session creation fails with a clear error. The snapshot must be built with `packer/build-images.sh` before the scenario is usable.
- The `playground: true` convention for open-ended scenarios is unchanged.
