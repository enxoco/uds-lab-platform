# Step 2 – Write the Helm chart

Helm is the standard packaging format for Kubernetes applications and the mechanism Zarf uses to deploy workloads. You need three files: `Chart.yaml`, `templates/deployment.yaml`, and `templates/service.yaml`. A `values.yaml` with the image reference and domain is already provided.

Start by moving into the working directory:

```
cd /root/myapp
```

## Chart.yaml

```
cat > chart/Chart.yaml << 'EOF'
apiVersion: v2
name: myapp
description: My UDS-packaged Python app
type: application
version: 0.1.0
appVersion: dev
EOF
```

## Deployment

```
cat > chart/templates/deployment.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: {{ .Values.image }}
          ports:
            - containerPort: 8080
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
EOF
```

The `readinessProbe` tells Kubernetes not to route traffic to the pod until `/health` returns 200. Kubernetes and the UDS uptime check (which you'll declare in step 3) both target this endpoint.

`{{ .Values.image }}` resolves to `myapp:dev` from `values.yaml`. At deploy time, Zarf replaces the image reference in the manifest with the cluster's internal Zarf registry — so the image reference in the running pod will differ from what you wrote here.

## Service

```
cat > chart/templates/service.yaml << 'EOF'
apiVersion: v1
kind: Service
metadata:
  name: myapp
  namespace: {{ .Release.Namespace }}
spec:
  selector:
    app: myapp
  ports:
    - port: 8080
      targetPort: 8080
EOF
```

The Service is a ClusterIP — it's only reachable inside the cluster. The UDS Package CR you'll write in step 3 tells Pepr to create an Istio VirtualService that routes external traffic from `myapp.uds.dev` to this Service.

## Render and check

```
uds zarf tools helm template test chart/
```

You should see a Deployment and Service rendered to YAML with no errors.

## Verify

```
uds zarf tools helm template test /root/myapp/chart/ 2>/dev/null | grep "kind:" | sort
```
