# ADR-0007: SQLite for Persistence

**Status:** Accepted

## Context

The platform currently stores all state in-memory. This was acceptable for alpha (no auth, no quotas, no user history) but is insufficient for production:

- User records must survive server restarts
- Quota usage must be durable (Hetzner bills don't reset on crash)
- Session state must be recoverable (prevents orphaned VMs after restart)
- Admin operations (org quota overrides, learning path assignments) need persistent storage

The platform must work in two deployment modes:
1. **Standalone binary** on a single server (simplest ops, no k8s)
2. **Kubernetes pod** on UDS Core with a PersistentVolumeClaim

Alternatives considered:
- **PostgreSQL**: Operationally correct for HA, but adds an external service dependency that conflicts with the single-binary deployment model and the goal of minimal complexity.
- **Redis**: Good for session caching but not a general-purpose relational store; would require a second dependency alongside a DB.
- **LibSQL / Turso**: SQLite-compatible with replication. Future migration path if HA becomes a requirement; premature for v1.
- **In-memory with periodic flush**: Fragile; loses data between flush intervals.

## Decision

Use embedded SQLite (`modernc.org/sqlite`, pure Go — no CGO dependency). Database file path is configurable via `DB_PATH` env var (default: `./lab.db`).

- **Standalone**: file on host disk
- **Kubernetes**: PVC mounted at `$DB_PATH`; single replica pod (no concurrent writers)

Schema (v1):
```sql
CREATE TABLE IF NOT EXISTS users (
    sub TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    org TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS org_quotas (
    org TEXT PRIMARY KEY,
    used_minutes INTEGER NOT NULL DEFAULT 0,
    override_minutes INTEGER,        -- NULL = use DEFAULT_ORG_MINUTES env var
    reset_at DATETIME
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_sub TEXT NOT NULL,
    scenario_id TEXT NOT NULL,
    vm_id INTEGER,
    status TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    minutes_used INTEGER NOT NULL DEFAULT 0
);
```

Migrations use `CREATE TABLE IF NOT EXISTS` for v1. A proper migration tool is deferred until schema churn warrants it.

## Consequences

- No external service dependency — binary + one file covers all persistence needs.
- Works identically in standalone and k8s single-replica deployments.
- Single replica only. Concurrent writes from multiple pods would corrupt the DB. If HA is ever required, migrate to LibSQL (SQLite-compatible, minimal code change).
- Server restart no longer orphans VMs — session state is recovered from DB on startup; cleanup loop reconciles against Hetzner API.
