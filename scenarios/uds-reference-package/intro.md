# UDS Reference Package

The [UDS Reference Package](https://github.com/uds-packages/reference-package) is the canonical example maintained by Defense Unicorns for how ISVs structure UDS-compatible packages. It demonstrates every major UDS Core integration: Istio ambient mesh, Keycloak SSO, Postgres via the operator pattern, Prometheus monitoring, and Pepr policy enforcement.

In this lab you'll explore the package structure, understand why each layer exists where it does, deploy the full stack with `uds run dev`, and verify that Pepr wired up the mesh, SSO, and monitoring automatically — all from a single `Package` CR.

**What's already running:** UDS Core (Keycloak, Istio, Pepr) on a k3d cluster.  
**What you'll deploy:** `postgres-operator` + `reference-package`, wired together via a UDS bundle.

> The image cache was pre-warmed during lab initialization. The terminal is ready now — you can start while setup finishes in the background.
