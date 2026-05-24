# Step 4 – Build, deploy, and verify

## Build the package

```
cd /root/hello-uds && uds zarf package create . --confirm
```

Zarf pulls the `nginx:1.27` image into a local OCI cache and bundles it with the Helm chart into a `.tar.zst` archive:

```
ls -lh zarf-package-*.tar.zst
```

This archive is **air-gap ready** — it contains everything needed to deploy with zero internet access.

## Deploy

```
uds zarf package deploy zarf-package-hello-uds-amd64-0.1.0.tar.zst --confirm
```

Zarf:
1. Creates the `hello-uds` namespace
2. Pushes the bundled image into the in-cluster registry (no external pull at deploy time)
3. Runs `helm upgrade --install` with your values bridge applied
4. Applies `manifests/uds-package.yaml`

## Check the Package CR

Pepr reconciles the `Package` CR within a few seconds. Watch it:

```
kubectl get package hello-uds -n hello-uds -w
```

Hit `Ctrl-C` once you see `Phase: Ready`.

## Confirm generated resources

Pepr turned your one CR into real Kubernetes/Istio objects:

```
kubectl get virtualservice,networkpolicy,authorizationpolicy -n hello-uds
```

You should see a VirtualService routing `hello-uds.uds.dev` → your pod, plus NetworkPolicies and AuthorizationPolicies.

## Check the pod

```
kubectl get pods -n hello-uds
```

Note the **2/2** in the READY column — that's your nginx container plus the Istio sidecar proxy injected automatically.

## Hit the service

```
kubectl run curl-test --image=curlimages/curl:8.7.1 --restart=Never --rm -it \
  -- curl -sk https://hello-uds.uds.dev | head -20
```

You should see the nginx welcome page HTML.

## Iterate

```bash
# After code or chart changes, rebuild and redeploy:
uds zarf package create . --confirm
uds zarf package remove hello-uds --confirm
uds zarf package deploy zarf-package-hello-uds-amd64-0.1.0.tar.zst --confirm
```

> If `package remove` leaves the namespace in `Terminating`, that's a Pepr finalizer. Run:
> `kubectl patch namespace hello-uds -p '{"metadata":{"finalizers":[]}}' --type=merge`

## Verify

The step passes once the `hello-uds` Package CR reaches `Ready` status.
