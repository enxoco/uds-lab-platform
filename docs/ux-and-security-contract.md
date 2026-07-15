# UDS Lab Platform: UX and Security Contract

Status: Draft

This document records the user experience and security properties the platform
must preserve while scenarios and infrastructure evolve.

## Product Decision

The platform remains VM-first. Each learner session receives an ephemeral
KubeVirt VM containing the tools, terminal, browser, and scenario environment.

Namespace-only sessions are not a substitute for VM sessions at this time.
They may be reconsidered for low-risk demonstrations, but they must not weaken
the sandbox security contract or create a misleading impression of VM-level
isolation.

## User Experience Requirements

### No client installation

Learners use a browser. They should not need local Kubernetes, Docker, QEMU,
UDS, Zarf, or SSH clients.

### Integrated terminal and browser

The session provides:

- A browser terminal for command-line work.
- An optional graphical browser for services such as `sso.uds.dev`.
- Consistent access to services using the URLs and workflows documented by the
  scenario.

The graphical browser is part of the VM experience because it preserves DNS,
TLS, cookies, SSO redirects, and access to the internal UDS Core domains
without requiring learners to configure their host machine.

### Scenario-driven capabilities

Scenarios declare required capabilities rather than forcing every session to
use the largest image. At minimum, scenarios can declare:

- VM size tier.
- Browser required or not required.
- Inner Kubernetes/UDS cluster required or not required.
- Setup and verification behavior.

The platform may optimize images and provisioning later, but capability changes
must not change the learner workflow unexpectedly.

### Reproducible reset

Every session starts from a known golden image or snapshot. User changes are
ephemeral and must not affect another learner or future sessions.

### Clear lifecycle

The UI must make provisioning, ready, paused, expired, failed, and teardown
states understandable. Expiration must eventually remove compute, storage,
network exposure, and session-specific credentials.

## Security Non-Negotiables

Learners are assumed to be curious and adversarial. They may deliberately try
to exhaust resources, access another session, reach the parent cluster, escape
the sandbox, or persist data after expiration.

### Isolation

- A learner session must run in its own VM.
- Learner workloads must not share a VM, host PID namespace, host network
  namespace, host filesystem, or privileged container with another learner.
- A session must not receive parent-cluster administrator credentials.
- A compromised session must not be treated as an acceptable path to the
  Kubernetes control plane or other sessions.

### Resource containment

The platform must enforce limits outside the learner-controlled environment for
CPU, memory, disk, processes, VM count, network exposure, and session lifetime.
Container-level limits alone are insufficient because a learner controls the
guest OS and can launch long-running processes or nested workloads.

### Network containment

Network access must be explicit. The platform must define and test:

- Which UDS Core services are reachable from a VM.
- Which external destinations are reachable.
- Whether sessions can communicate with one another.
- Whether sessions can reach cloud metadata, the Kubernetes API, node services,
  registry credentials, or management endpoints.

### Browser containment

The graphical browser must run inside the session VM, without host browser
integration or host filesystem access. Browser access must not expose the
platform's Kubernetes credentials, operator credentials, or other sessions.

### Cleanup and retention

Session expiration must remove VM compute and session-specific storage and
network objects. Any retained history must be limited to the minimum metadata
needed for the user experience, auditing, or support and must not retain the
guest disk or secrets unintentionally.

### Observability without secret leakage

Operational logs may record lifecycle and health information, but must not
capture learner command input, tokens, passwords, private keys, or arbitrary
guest console output unless explicitly required and protected.

## Deliberately Out of Scope for Namespace-Only Sessions

The following are not acceptable reasons to replace a VM with a namespace:

- Lower provisioning cost alone.
- Easier M4 Mac support alone.
- Avoiding image-build or KubeVirt work.
- Assuming UDS Core policy enforcement is equivalent to a tenant boundary.
- Assuming Kubernetes RBAC prevents kernel, runtime, or control-plane escape.

Namespace-only sessions would require a separate threat model, admission
policy, quota model, network model, browser architecture, and penetration test.

## Decisions Still Needed

- Maximum session lifetime and behavior after expiration.
- Whether pause/resume is required for every scenario.
- Whether learners may access the public internet and which destinations are
  allowed.
- Whether browser sessions must support downloads, clipboard, file upload, or
  copy/paste.
- Required provisioning-time target for each VM size tier.
- Whether session disk state is ever retained after expiration.
- Which scenarios are allowed to run an inner Kubernetes cluster.
- Required evidence before changing the VM-first decision.

## Acceptance Bar for a Future Architecture Change

Any alternative to the VM model must demonstrate, before rollout:

1. A written threat model for hostile learners.
2. Automated tests for cross-session access and resource exhaustion.
3. Tests for Kubernetes API, node, metadata-service, and secret exposure.
4. Browser UX parity for internal UDS Core services and SSO.
5. Deterministic cleanup and recovery after learner-controlled workloads hang.
6. A documented residual-risk decision approved by the platform owner.
