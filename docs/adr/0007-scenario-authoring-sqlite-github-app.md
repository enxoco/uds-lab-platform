# ADR-0007: Scenario Authoring — SQLite Store, Snapshot Versioning, GitHub App Integration

**Status:** Accepted  
**Date:** 2026-06-07

---

## Context

Scenarios are currently baked into the labserver binary via `//go:embed scenarios` — read-only at runtime. Scenario Authors must edit files locally, rebuild, and redeploy to change content. This blocks less technical contributors and makes mid-workshop corrections slow.

The goal is a web-based authoring UI accessible to any Admin, with GitHub as an optional distribution and version-control channel.

---

## Decisions

### 1. SQLite for mutable scenario storage

Scenarios are moved from the embedded FS into a SQLite database at runtime (`DB_PATH` env var, default `/data/lab.db`). The embedded `scenarios/` directory seeds the DB on first boot and is otherwise unused at runtime.

**Alternatives considered:**
- **Disk directory with volume mount** — simpler, but no versioning or draft/publish workflow without additional tooling.
- **PostgreSQL** — unnecessary operational overhead for a single-server workshop tool.

SQLite adds no network dependency, ships as a single file, and is trivially backed up. The embedded FS remains in the binary as a bootstrap seed.

### 2. Whole-scenario snapshot versioning

Each save operation writes a new `scenario_versions` row containing a full copy of all scenario files. Rollback replaces the current state with the snapshot. Admins can view version history and restore any prior version.

**Alternatives considered:**
- **Per-file history** — more granular but requires reconstructing a coherent cross-file state at a point in time. A step markdown rolled back independently of its verify script creates inconsistency. Whole-scenario snapshots eliminate this class of problem.

Scenarios are small (text files, rarely >50KB total). Storage redundancy is negligible.

### 3. Draft/Publish workflow

Scenarios have a `published` boolean flag. Participants only see Published scenarios. All edits, including GitHub Pull imports, land in Draft state. Admins explicitly publish when ready. This prevents mid-edit content from appearing in the participant catalog.

### 4. Shellcheck gates script saves (replaces human warning)

`setup.sh` and `verify/*.sh` are shell scripts that run on live VMs. Rather than warning users or restricting script editing, saves are blocked if `shellcheck` reports errors. `shellcheck` is installed in the labserver Docker image and invoked server-side on script content before any DB write. Snapshot rollback is the safety net for logic errors shellcheck cannot catch.

### 5. GitHub App over Personal Access Token

GitHub integration uses a GitHub App (App ID + PEM private key + installation ID) rather than a Personal Access Token.

**Rationale:**
- PATs are user-scoped and die when the user leaves. GitHub Apps are org-level actors.
- GitHub App permissions are fine-grained (contents read/write on specific repos only).
- GitHub Apps support webhook delivery, enabling auto-pull on push without polling.
- The interface (`ScenarioRepoClient`) is designed for swap-in replacement — PAT could have been used initially, but GitHub App complexity is front-loaded once rather than paid as a migration later.

**Env vars:** `GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY` (base64 PEM), `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_WEBHOOK_SECRET`.

### 6. Pull-in + Export model (not bidirectional sync)

GitHub integration is two explicit one-directional operations — not a sync loop:
- **Scenario Pull**: imports from GitHub repo → new Draft versions in DB.
- **Scenario Export**: pushes current DB state → commit on GitHub repo.

No conflict resolution is needed. Pull always creates new versions (rollback available). Export is always an explicit Admin action. This avoids the complexity of a sync loop while preserving both a git-native and UI-native workflow.

### 7. Monaco editor with live preview (not rich text WYSIWYG)

Step markdown is edited in Monaco with a side-by-side rendered preview. Click-to-Run code blocks require exact fence syntax; a rich text editor (TipTap, Quill) would require a custom extension to round-trip these correctly and introduces a new large dependency. Monaco is already bundled for the participant IDE.

---

## Consequences

- SQLite adds a `modernc.org/sqlite` (pure Go, CGO-free) or `mattn/go-sqlite3` (requires CGO) dependency. Prefer `modernc.org/sqlite` to keep CGO-free builds.
- Docker Compose must mount a volume at `/data` for scenario persistence across container restarts.
- `shellcheck` binary must be present in the Docker image.
- GitHub App creation is a one-time setup step for operators who want GitHub integration. The feature degrades gracefully when `GITHUB_APP_ID` is unset (pull/export UI hidden).
- The embedded `scenarios/` FS is retained in the binary as a bootstrap seed. It is not a fallback at runtime — the DB is always authoritative after first boot.
