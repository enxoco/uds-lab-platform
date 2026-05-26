# ADR-0002: Cookie-Based Client Identity with No Authentication

**Status:** Accepted (alpha); GitHub OAuth planned

## Context

The platform needs to identify users to enforce one active Session per person and to enable future per-user features (history, quotas). Full authentication (OAuth, SSO) adds meaningful complexity and external dependencies.

Alternatives considered:
- **GitHub OAuth**: Correct long-term solution. Ties identity to a real user account, enables per-user session history and quotas. Deferred — too much complexity for alpha.
- **Anonymous session (no identity at all)**: No enforcement possible; users could spin up unlimited VMs.
- **IP-based identity**: Unreliable behind NAT/VPN; breaks in shared office environments.

## Decision

Generate a random UUID cookie (`lab_client_id`, HttpOnly, 30-day expiry) on first visit. This cookie is the Client identity. Enforce one active Session per Client server-side. No login required.

## Consequences

- Zero friction for users — no signup, no login.
- Identity is browser-scoped, not person-scoped. One person using two browsers = two Clients = two independent session limits.
- Session ID lives in the URL (not the cookie), so closing a tab without bookmarking creates an Orphaned Session with no resume path.
- No per-user history, analytics, or quotas possible with this model.
- Migration to GitHub OAuth will require mapping existing cookie-scoped sessions to user accounts or simply expiring them.
