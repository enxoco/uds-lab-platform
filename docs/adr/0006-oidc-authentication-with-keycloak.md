# ADR-0006: OIDC Authentication with Keycloak as Default Identity Provider

**Status:** Accepted

## Context

ADR-0002 deferred authentication in favor of a cookie-based Client identity for alpha. The platform now needs real user authentication to support:

- Per-user session enforcement (not per-browser)
- Org-level quota tracking and lab minute allocation
- Admin access control for DU staff
- Telemetry tied to stable user identities (anonymized)

The user audience is mixed: external ISVs (often with GitHub or GitLab accounts) and internal DU employees (who authenticate via DU's existing Keycloak/SSO infrastructure). GitHub-only OAuth is insufficient because not all ISVs use GitHub.

Alternatives considered:
- **GitHub OAuth only**: Zero IDP infra, but excludes GitLab and non-GitHub ISVs. Not viable.
- **Username/password**: Requires credential management, password reset flows, account provisioning. Too much complexity.
- **Multiple direct OAuth integrations (GitHub + GitLab + Google)**: N separate OAuth flows to maintain; no unified identity layer.

## Decision

The application speaks OIDC to a single configurable provider (env: `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`). The default deployed IDP is Keycloak, which federates external identity providers (GitHub, GitLab, Google) for ISVs and connects to DU's internal SSO for employees. The app remains provider-agnostic — it validates JWTs, extracts `sub`, `email`, and `org` (from a custom claim or `groups`), and never knows which upstream provider the user authenticated with.

Admin access is gated by a Keycloak group (`lab-platform-admins`). The app checks the `groups` claim in the JWT. DU controls membership in Keycloak.

On first login, a User record is created in the local DB (`sub`, `email`, `org`). The `sub` claim is the stable internal identifier. Email is stored for admin visibility. For telemetry, `sha256(sub)` is used as the anonymous user key — email never appears in event data.

## Consequences

- ISVs can authenticate with GitHub, GitLab, or Google — no GitHub requirement.
- DU employees use existing SSO — no separate account.
- App has no auth logic to maintain; IDP handles it.
- Requires Keycloak to be deployed and configured (or another OIDC provider via env vars for standalone deployments).
- Replaces the `lab_client_id` cookie model from ADR-0002. Existing alpha sessions are not migrated — they expire naturally.
- One active Session per User (person-scoped, not browser-scoped). Orphaned Session problem is resolved: any authenticated browser can resume a running Lab.
