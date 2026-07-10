# Lab Complete

You've deployed the UDS Reference Package end-to-end and seen every major UDS integration in action:

- **Zarf** scoped the package to exactly one thing: the application. No Postgres, no Keycloak, no Istio — those are someone else's responsibility.
- **The bundle** composed `postgres-operator` and `reference-package` at the right layer, with full override capability exposed to the operator deploying it.
- **The UDS Package CR** declared what the app needs — exposed port, egress to Postgres and Keycloak, SSO client config — and Pepr turned that into VirtualServices, NetworkPolicies, AuthorizationPolicies, ServiceMonitors, and a Keycloak client registration automatically.
- **Ambient mesh** enforced mTLS and captured metrics at the node level via ztunnel — no sidecar injection required.

This is the reference pattern. When you author your own UDS package, follow this exact structure: application code in the Zarf package, infrastructure operators in the bundle, network and SSO policy in the UDS Package CR.

**Next:** Try *Build Your Own UDS Package* — start from a Python Flask app and author every layer yourself from scratch.
