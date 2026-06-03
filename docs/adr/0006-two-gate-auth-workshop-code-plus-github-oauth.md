# ADR-0006: Two-Gate Authentication — Workshop Code + GitHub OAuth

**Status:** Accepted

**Supersedes:** ADR-0002 (Cookie-Based Client Identity with No Authentication)

## Context

The platform needs to gate access for workshop events: distribute the URL to participants only, prevent unauthorized attendees from spinning up VMs, and give organizers visibility into active sessions with the ability to terminate them.

Requirements:
- Any valid GitHub account is sufficient identity (no org membership check)
- A pre-shared Workshop Code provides a first line of defense against URL guessing
- Organizers need an admin view to see who is active and boot anyone out
- Minimal new infrastructure — the existing `lab_client_id` cookie and in-memory session map should be extended, not replaced

Alternatives considered:
- **GitHub org membership check**: Rejected — workshop participants may not be org members; adds friction and failure modes.
- **Allowlist of specific GitHub usernames**: Rejected — too much organizer overhead before each workshop; Workshop Code achieves the same goal with less friction.
- **Persistent auth storage (DB/Redis)**: Rejected — a server restart requiring re-auth is acceptable for a workshop context; no new infrastructure warranted.
- **JWT-based stateless auth**: Rejected — adds signing key management with no benefit over extending the existing in-memory client map.

## Decision

Add a two-step Auth Gate before any protected route:

1. **Workshop Code** — user submits `WORKSHOP_CODE` env var value on `/login`. Server sets a short-lived cookie (`lab_code_ok`) on success. Code check happens before GitHub OAuth to fail fast without a round-trip.
2. **GitHub OAuth** — standard OAuth 2.0 flow (`/auth/github` → GitHub → `/auth/callback`). On success, GitHub username is attached to the Client record in the in-memory session map.

Unauthenticated requests redirect to `/login`. Protected routes: `/`, `/lab.html`, `/api/*`.

Admin access (`/admin`) is restricted to GitHub usernames listed in `ADMIN_USERS` env var. Admins can view all active Sessions and terminate any Session (VM destroyed, client auth cleared).

New env vars:
- `WORKSHOP_CODE` — the gate passphrase
- `GITHUB_CLIENT_ID` — GitHub OAuth App client ID
- `GITHUB_CLIENT_SECRET` — GitHub OAuth App client secret
- `GITHUB_CALLBACK_URL` — full callback URL (e.g., `https://uds-labs.hackanooga.com/auth/callback`)
- `ADMIN_USERS` — comma-separated GitHub usernames with admin access

## Consequences

- Participants must have a GitHub account — acceptable for a developer-focused workshop.
- Auth state is in-memory; server restart requires re-auth. Active VM Sessions are unaffected (VMs keep running); only the auth cookie is invalidated.
- Workshop Code in env var means rotating it requires a redeploy (~30s). Acceptable for workshop cadence.
- Client identity remains browser-scoped — one person, two browsers = two Clients. GitHub username is attached per-Client, not deduplicated across browsers.
