# Build Your Own UDS Package

UDS packaging has a specific anatomy: a Helm chart delivers the workload, a Zarf package bundles the chart and its images, a UDS Package CR declares the network and SSO policy, and a UDS bundle composes everything for deployment.

In this lab you'll author every one of those layers from scratch, using a pre-containerized Python Flask app as your subject. By the end, `uds run dev` will build your package, create the bundle, and deploy the app to the running UDS Core cluster — reachable at `myapp.uds.dev` via the Istio ingress gateway.

**What's already running:** UDS Core (Keycloak, Istio, Pepr) on a k3d cluster. The app Docker image (`myapp:dev`) is building in the background — it will be ready by step 4.  
**What you'll write:** Helm chart, UDS Package CR, zarf.yaml, bundle, and tasks.yaml.

> All files live in `/root/myapp`. That's your working directory for this lab.
