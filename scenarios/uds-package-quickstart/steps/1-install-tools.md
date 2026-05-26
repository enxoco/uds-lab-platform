# Step 1 – Install prerequisites

Before packaging an app with UDS you need Docker, k3d, and the UDS CLI. Run each block below — click any code block to send it to the terminal.

## Install Docker CE

```
apt-get update && apt-get install -y ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
```

```
. /etc/os-release && tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${UBUNTU_CODENAME:-$VERSION_CODENAME}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
```

```
apt-get update && apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

Verify:

```
docker version --format '{{.Server.Version}}'
```

## Install k3d

k3d creates lightweight Kubernetes clusters using Docker as the container runtime.

```
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
```

```
k3d version
```

## Install UDS CLI

UDS CLI wraps Zarf and adds UDS-specific bundle and deploy commands.

```
UDS_TAG=$(curl -s https://api.github.com/repos/defenseunicorns/uds-cli/releases/latest | grep '"tag_name"' | cut -d'"' -f4) && curl -sSL "https://github.com/defenseunicorns/uds-cli/releases/download/${UDS_TAG}/uds-cli_${UDS_TAG}_Linux_amd64" -o /usr/local/bin/uds && chmod +x /usr/local/bin/uds
```

```
uds version
```

## Kubectl alias

```
echo 'alias kubectl="uds zarf tools kubectl"' >> ~/.bashrc && echo 'alias k="uds zarf tools kubectl"' >> ~/.bashrc && source ~/.bashrc
```

## Verify

When `docker version`, `k3d version`, and `uds version` all print output, click **Next** to continue.
