# UDS Lab Platform — Domain Glossary

Canonical terms for this codebase. When in doubt, use these names exactly.

---

## Core Concepts

### Lab
The live, running experience a user interacts with. A Lab is a Scenario or Playground instantiated as a Session — it is the combination of content definition and a running VM environment. "Lab" is the user-facing word; users start, use, and end Labs.

### Scenario
A guided, linear, step-by-step Lab. The static definition: YAML metadata, ordered Steps, a Setup Script, and optional Verify scripts. Scenarios have a defined learning objective and a defined end state. Users progress through Steps in order; Steps with Verify must be passed before advancing. Author-facing artifact stored as a directory in `scenarios/`.

### Playground
An open-ended Lab with pre-installed tools and no required completion path. A Playground provides a pre-built environment for exploration and practice — not structured learning toward a specific outcome. Distinguished from a Scenario by intent: Playgrounds are "choose your own adventure," Scenarios are guided curriculum.

### Session
The technical lifecycle handle for a running Lab: VM ID, VM IP, status, TTL, and Client binding. One Session per Client enforced. Code-facing term — users experience a Lab, the system manages a Session.

### Step
A unit of instruction within a Scenario. Each Step contains markdown content rendered in the left panel. Steps may optionally include a Verify script. Without Verify, users advance freely. With Verify, users must pass the check before proceeding.

### Client
A browser-scoped identity derived from a `lab_client_id` cookie (HttpOnly, 30-day expiry). A Client is a browser/device, not necessarily a person — one person using two browsers has two independent Clients. One active Session per Client is enforced. **Not** a User identity: there is no authentication.

---

## VM Images

### Base Image
The foundation image all other images build upon. Contains the full platform runtime: Ubuntu, tmux, ttyd, Chromium, noVNC, systemd service units, and the injection server. Every Lab VM starts from Base Image or an image layered on top of it.

### Playground Image
A VM image layered on top of Base Image with heavy tooling pre-installed (Docker, k3d, UDS CLI, etc.). Used by Playground Labs for fast startup — expensive tools are pre-baked rather than installed at boot. Playground Images stack in tiers (e.g., `tools` → `uds-core`).

---

## Terminal Experience

### Lab Terminal
The primary terminal tab (Tab 1) in the Lab UI. Attached to the tmux session on the VM. Shows live setup log output while the Setup Script runs, then transitions to an interactive prompt when the VM is ready. The target for Click-to-Run command injection. Only one Lab Terminal exists per Session.

### Shell Terminal
Additional terminal tabs (Tab 2+) in the Lab UI. Direct root bash shells, always available — even while the Setup Script is still running. Users open Shell Terminals to explore the VM or run commands without waiting for setup to complete. Multiple Shell Terminals can be open simultaneously. Shell Terminals are not Click-to-Run targets; clicking a code block while a Shell Terminal is active automatically switches focus to the Lab Terminal before injecting the command.

---

## Authoring Concepts

### Setup Script
A bash script (`setup.sh`) bundled into each Scenario. Runs on VM boot in the background. Responsible for preparing the environment (installing packages, starting services, configuring the cluster, etc.). Signals completion by touching `/var/log/lab-setup/ready`, which unblocks the Lab Terminal.

### Verify
A per-Step bash script that validates the user has completed a Step correctly. Exit 0 = pass; non-zero = fail. Run with a 30-second timeout. When a Step has a Verify script, the "Next" button is blocked until the check passes. Verify scripts are optional per Step.

### Click-to-Run
Code blocks in Step markdown that, when clicked in the Lab UI, inject the command directly into the Lab Terminal via tmux `send-keys`. No confirmation prompt — these are ephemeral environments. Click-to-Run blocks are the primary mechanism for guiding users through hands-on steps.

### Scenario Author
An engineer who writes Scenarios or Playgrounds. Currently an internal Defense Unicorns role; intent is to open this to external contributors as the platform matures.

---

## Reference Artifacts

### Reference Package
The canonical UDS package maintained by Defense Unicorns at `github.com/uds-packages/reference-package`. Demonstrates best practices for structuring a UDS package: app-only `zarf.yaml`, infrastructure dependencies at the bundle layer, UDS Package CR with SSO/network/monitoring, and `uds run dev` as the development workflow. ISVs use it as the authoritative template.

---

## Services & Browser

### Service
A clickable chip in the Lab UI that opens a URL in the VM's browser. Services are defined two ways — statically in `scenario.yaml` (known at authoring time) or auto-detected from live Istio VirtualServices on the cluster at runtime. Both merge into the same UI element; the distinction is an implementation detail.

### Browser Mode
A Lab configuration (`browser: true` in scenario.yaml) that starts a full desktop browser environment inside the VM (Xvfb + x11vnc + noVNC + Chromium). Architecturally necessary for UDS scenarios: `*.uds.dev` resolves to `127.0.0.1`, so UDS Core services are only reachable from within the VM itself. The VM browser is always Chromium (baked into Base Image).

---

## Session Lifecycle

### Session Expiry
Automatic deletion of a Session and its VM when the TTL elapses (default: 60 minutes, configurable). The UI displays a countdown timer (Lab Timeout). Users cannot extend a Session — known limitation in the current alpha. The VM is deleted immediately on expiry; any unsaved work is lost.

### Orphaned Session
A Session that is active on the server but inaccessible to the user because the lab URL (which carries the Session ID) has been lost. The Client cookie still exists, so the server blocks new Session creation, but the catalog offers no resume path. Known limitation of the current Client identity model.

---

## Known Limitations (Alpha)

- **No authentication** — identity is browser-scoped (Client), not user-scoped. No login, no per-user history.
- **No session resume** — Session ID lives in the URL only; closing the tab without bookmarking creates an Orphaned Session.
- **No session extension** — Session Expiry is hard; TTL cannot be extended once a Session starts.
- **One session per Client** — a Client cannot run multiple Labs simultaneously.
- **Click-to-Run during active setup** — Click-to-Run always targets the Lab Terminal (Tab 1). If the Setup Script is still running, the command is buffered by tmux and fires silently once the terminal becomes interactive. No warning is shown; users may not realize their command was queued.
