# Step 6 – Deploy and verify

```
cd /root/myapp && uds run dev
```

This runs the three commands from your `tasks.yaml` in sequence. Watch the output:

1. **Package create** — Zarf finds `myapp:dev` in the local Docker daemon, packages the image layers and Helm chart into `zarf-package-myapp-amd64-dev.tar.zst`
2. **Bundle create** — UDS wraps the Zarf package into `uds-bundle-myapp-bundle-amd64-dev.tar.zst`
3. **Bundle deploy** — UDS deploys the bundle: runs Helm to create the `myapp` namespace and Deployment, then Pepr begins reconciling the Package CR

## Watch the rollout

Open a second terminal tab and monitor:

```
watch uds zarf tools kubectl get pods -n myapp
```

Wait for the pod to reach `Running` with `1/1` READY. With ambient mesh, `1/1` is correct — no Istio sidecar is injected alongside your container.

## Check Pepr's work

Once the pod is Running, check what Pepr generated from your Package CR:

```
uds zarf tools kubectl get package myapp -n myapp -w
```

Hit `Ctrl-C` when Phase reaches `Ready`. Then inspect the generated resources:

```
uds zarf tools kubectl get virtualservice,networkpolicy,authorizationpolicy -n myapp
```

You should see:
- A `VirtualService` routing `myapp.uds.dev` to your Service
- `NetworkPolicy` resources matching your two `allow` rules
- `AuthorizationPolicy` resources for mesh-level access control

Pepr generated all of these from the 30-line `uds-package.yaml` you wrote in step 3. In a manual setup you'd write each of these objects by hand and keep them synchronized across environments.

## Package CR status

```
uds zarf tools kubectl get package myapp -n myapp -o jsonpath='{.status.phase}'
```

Output: `Ready`

## List all your artifacts

```
ls /root/myapp/*.tar.zst /root/myapp/uds-bundle-*.tar.zst 2>/dev/null
```

The `.tar.zst` files are self-contained deployment artifacts. Either one can be copied to a disconnected environment and deployed with `uds deploy` — no cluster or internet access needed for the image.

## Bootstrap the Keycloak user

The app is behind Keycloak SSO. Before you can reach it in the browser, create the test user:

```
cd /root/myapp && uds run setup:keycloak-user --with group="/UDS Core/Admin"
```

This creates **doug@uds.dev** in Keycloak with password `unicorn123!@#UN`.

## Open the app in the browser

Once the pod is Running and the Keycloak user is created, click the **myapp** service chip that appears in the browser panel above. You'll be redirected to Keycloak — log in as `doug@uds.dev` with password `unicorn123!@#UN`.

You should land on the "Hello from UDS!" page. The request traveled through:

1. Istio ingress gateway — TLS termination at the tenant gateway
2. Pepr-generated VirtualService — routes `myapp.uds.dev` to the `myapp` Service
3. **AuthService** — intercepts the request, validates the Keycloak session, forwards if valid
4. ztunnel — mTLS within the ambient mesh to your pod
5. Flask serving the response

Your Flask app has zero authentication code. AuthService handled the full OIDC flow on its behalf, driven entirely by the `enableAuthserviceSelector` field in the Package CR you wrote in step 3.

## Verify

```
export KUBECONFIG=/root/.kube/config
uds zarf tools kubectl get pods -n myapp --no-headers 2>/dev/null | awk '$3=="Running"' | wc -l
```
