# ADR-0009: OpenTelemetry for Observability

**Status:** Accepted

## Context

The platform currently has no structured observability — only `log.Printf` calls. For production, DU needs:

- Usage analytics: which Scenarios are being used, completed, abandoned
- ISV progress tracking: did Acme complete their Learning Path?
- Operational health: VM provisioning latency, error rates, active session count
- Cost signals: quota usage ratios per org

The platform runs in two modes: standalone server and Kubernetes on UDS Core. UDS Core ships Grafana + Prometheus + Loki — the observability stack is already present in k8s deployments. Standalone deployments need a path that works without that stack.

Alternatives considered:
- **Prometheus client only**: Metrics only, no structured logs or traces. Insufficient for event-level analytics (step completions, scenario starts by org).
- **Custom analytics DB**: Bespoke event log schema. Reinvents the wheel; no standard tooling.
- **Third-party SaaS (Datadog, Mixpanel)**: External data egress, vendor lock-in, potential PII concerns.

## Decision

Use OpenTelemetry (`go.opentelemetry.io/otel`) for all three signal types:

**Metrics** (Prometheus exporter, scrape endpoint at `/metrics`):
- `lab_sessions_started_total{scenario, org}` — counter
- `lab_sessions_completed_total{scenario, org}` — counter (completed = user reached final Step)
- `lab_sessions_abandoned_total{scenario, org}` — counter (expired or deleted before final Step)
- `lab_session_duration_minutes{scenario}` — histogram
- `lab_steps_verified_total{scenario, step}` — counter (tracks where users get stuck)
- `lab_verify_attempts_per_step{scenario, step}` — histogram (retries before pass)
- `lab_quota_usage_ratio{org}` — gauge (used/total)
- `lab_active_sessions` — gauge
- `lab_vm_provision_duration_seconds{scenario}` — histogram

**Structured logs** (JSON to stdout; scraped by Loki in k8s; readable standalone):
- All HTTP requests: method, path, status, latency, `user_key` (sha256 of sub — no PII)
- Session lifecycle events: start, end, status transitions
- Quota events: warn threshold crossed, block triggered

**Traces** (OTLP export, optional):
- VM provisioning span (covers Hetzner API call + polling until ready)
- Useful for diagnosing slow cold starts

**No PII in telemetry**: `user_key = sha256(sub)` in all events. `org` is company-level and not personal data. Email never appears outside the users DB table.

Configuration via standard OTel env vars:
- `OTEL_EXPORTER_OTLP_ENDPOINT` — set for k8s/UDS Core deployments; unset disables OTLP export
- Prometheus scrape endpoint always active (zero config)
- `OTEL_SERVICE_NAME` defaults to `uds-lab-platform`

In UDS Core deployments, the in-cluster OTel collector receives OTLP data and routes to Prometheus + Loki + Tempo. Grafana dashboards are provisioned as part of the UDS Package for this platform.

In standalone deployments, Prometheus scrapes `/metrics` directly. Structured logs go to stdout for the operator to handle.

## Consequences

- Single observability approach works for both deployment modes.
- No vendor lock-in; OTel is the industry standard.
- Step-level verification metrics give Scenario Authors actionable signal on where learners struggle.
- Org-level completion rates give DU visibility into ISV onboarding progress.
- PII-free telemetry design: safe to retain event data long-term, safe to share in dashboards.
