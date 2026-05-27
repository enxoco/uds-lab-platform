# ADR-0008: Org-Level Quota Model and Learning Paths

**Status:** Accepted

## Context

Without quotas, any authenticated user can spin up unlimited Hetzner VMs. Each VM costs real money (ccx13 ~€0.013/hr, ccx23 ~€0.032/hr). DU needs cost control and visibility into how ISVs engage with the platform.

Additionally, ISV onboarding is a structured process: DU recommends specific Scenarios to ISVs based on onboarding context. The platform should surface a curated learning experience without restricting catalog access.

## Decision

### Quota

Quota is enforced at the **Organization** level, not per-user. All users in an org share a single lab-minutes pool.

- Default allocation: `DEFAULT_ORG_MINUTES` env var (e.g., `300`)
- Per-org override: `org_quotas.override_minutes` in DB (NULL = use default)
- **Warn at 80%**: UI and API response include `quota_warning: true` when org has consumed ≥80% of allocation
- **Hard block at 100%**: `POST /api/sessions` returns 429 when org is at or above allocation. Existing running Sessions are never killed mid-lab.
- Minutes consumed = actual session duration, tracked at Session end (or cleanup) by diffing `started_at` / `ended_at`
- Quota reset period: manual by DU admin via admin API (`POST /api/admin/orgs/:org/quota/reset`). No automatic monthly reset in v1.

DU manages quotas via an admin page gated by Keycloak group `lab-platform-admins`.

### Learning Paths

A **Learning Path** is a named, ordered list of Scenarios that DU recommends for a category of user (e.g., "Standard ISV Onboarding", "DU Employee Onboarding"). Learning Paths are defined in `learning-paths.yaml` config (not DB) for v1.

- The full Scenario catalog is always visible and accessible to all authenticated users.
- An org is associated with one Learning Path (set by DU admin; stored in `org_quotas` table or separate `org_config` table).
- The catalog UI surfaces the org's Learning Path scenarios at the top with a progress indicator.
- Completion is observational — tracked via telemetry (session completed = user reached final Step). No enforcement.

Alternatives considered:
- **Per-user quota**: Users within an org are independent; harder to manage and doesn't match how DU thinks about ISV relationships ("Acme gets 300 minutes").
- **Hard scenario assignment** (blocking catalog access): Overly restrictive; ISVs should be free to explore.
- **Per-org custom paths in DB**: Flexible but premature; standard paths cover the v1 use case. Revisit when per-org customization is requested.

## Consequences

- DU can manage ISV lab spend at the engagement level (per org), not per individual.
- ISVs get a guided experience (Learning Path) without losing catalog freedom.
- Learning Path config in YAML means changes require redeploy (acceptable for v1; migrate to DB if DU needs live updates).
- No automatic quota reset — DU must manually reset. Adds an admin task but gives DU full control over billing cycles.
