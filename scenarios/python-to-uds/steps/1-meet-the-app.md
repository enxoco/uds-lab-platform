# Step 1 – The app: Python Flask on port 8080

Your working directory is `/root/myapp`. The app, its `requirements.txt`, and its `Dockerfile` are already here — provided so you can focus on the UDS packaging layers rather than application code.

## Read the app

```
cat /root/myapp/app.py
```

The app does exactly two things:
- `GET /` — returns an HTML page
- `GET /health` — returns `{"status": "ok"}`

Port **8080** is the one number you'll see in every layer of the stack: the Dockerfile, the Helm deployment, the Kubernetes Service, the UDS Package CR, and the Zarf chart definition.

## Read the Dockerfile

```
cat /root/myapp/Dockerfile
```

| Layer | Why |
|-------|-----|
| `FROM python:3.11-slim` | Small base image with Python already installed |
| `RUN pip install -r requirements.txt` | Flask installed at image build time |
| `EXPOSE 8080` | Documents the port (informational — doesn't open anything) |
| `CMD ["python", "app.py"]` | Starts the Flask server when the container runs |

## The image is being built in the background

The lab setup is building the Docker image (`myapp:dev`) in the background. Check when it's ready:

```
docker image ls myapp:dev
```

When you write `zarf.yaml` in step 4, you'll reference this image by name (`myapp:dev`). Zarf looks up the image from the local Docker daemon, bundles all its layers into a `.tar.zst` archive, and from that point forward no registry access is needed — the image travels with the package.

## Verify

```
ls /root/myapp/app.py /root/myapp/requirements.txt /root/myapp/Dockerfile
```
