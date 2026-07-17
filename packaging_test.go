package labplatform_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type zarfPackage struct {
	Components []struct {
		Name   string   `yaml:"name"`
		Images []string `yaml:"images"`
		ImageArchives []struct {
			Images []string `yaml:"images"`
		} `yaml:"imageArchives"`
		Manifests []struct {
			Files []string `yaml:"files"`
		} `yaml:"manifests"`
		Actions struct {
			OnDeploy struct {
				After []struct {
					Cmd  string `yaml:"cmd"`
					Wait struct {
						Cluster struct {
							Kind      string `yaml:"kind"`
							Name      string `yaml:"name"`
							Namespace string `yaml:"namespace"`
							Condition string `yaml:"condition"`
						} `yaml:"cluster"`
					} `yaml:"wait"`
				} `yaml:"after"`
			} `yaml:"onDeploy"`
		} `yaml:"actions"`
	} `yaml:"components"`
}

type udsPackageManifest struct {
	Spec struct {
		Network struct {
			Allow []udsNetworkAllow `yaml:"allow"`
		} `yaml:"network"`
	} `yaml:"spec"`
}

type udsNetworkAllow struct {
	Direction       string            `yaml:"direction"`
	Selector        map[string]string `yaml:"selector"`
	RemoteNamespace string            `yaml:"remoteNamespace"`
	RemoteSelector  map[string]string `yaml:"remoteSelector"`
	RemoteGenerated string            `yaml:"remoteGenerated"`
	Port            int               `yaml:"port"`
}

type applicationPackage struct {
	Metadata struct {
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Variables []struct {
		Name string `yaml:"name"`
	} `yaml:"variables"`
	Components []struct {
		Name   string   `yaml:"name"`
		Images []string `yaml:"images"`
		Charts []struct {
			Version string `yaml:"version"`
		} `yaml:"charts"`
	} `yaml:"components"`
}

type chartValues struct {
	Image string `yaml:"image"`
}

type chartMetadata struct {
	Version    string `yaml:"version"`
	AppVersion string `yaml:"appVersion"`
}

func TestVMImageComponentsInMainPackage(t *testing.T) {
	contents, err := os.ReadFile("zarf.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var pkg zarfPackage
	if err := yaml.Unmarshal(contents, &pkg); err != nil {
		t.Fatalf("parse package: %v", err)
	}

	var server, imports *struct {
		Name   string   `yaml:"name"`
		Images []string `yaml:"images"`
		ImageArchives []struct {
			Images []string `yaml:"images"`
		} `yaml:"imageArchives"`
		Manifests []struct {
			Files []string `yaml:"files"`
		} `yaml:"manifests"`
		Actions struct {
			OnDeploy struct {
				After []struct {
					Cmd  string `yaml:"cmd"`
					Wait struct {
						Cluster struct {
							Kind      string `yaml:"kind"`
							Name      string `yaml:"name"`
							Namespace string `yaml:"namespace"`
							Condition string `yaml:"condition"`
						} `yaml:"cluster"`
					} `yaml:"wait"`
				} `yaml:"after"`
			} `yaml:"onDeploy"`
		} `yaml:"actions"`
	}
	serverIdx := -1
	for i := range pkg.Components {
		switch pkg.Components[i].Name {
		case "vm-image-server":
			server = &pkg.Components[i]
			serverIdx = i
		case "golden-pvcs":
			imports = &pkg.Components[i]
		}
	}
	if server == nil || imports == nil {
		t.Fatal("root zarf.yaml must contain both vm-image-server and golden-pvcs components")
	}
	for i := range pkg.Components {
		if pkg.Components[i].Name == "golden-pvcs" && i <= serverIdx {
			t.Fatal("vm-image-server must come before golden-pvcs in component order")
		}
	}

	for _, image := range server.Images {
		if !strings.HasPrefix(image, "ghcr.io/enxoco/lab-vm-images/") {
			t.Fatalf("vm-image-server image must use ghcr.io/enxoco registry, got %q", image)
		}
	}
	if len(server.Images) != 2 {
		t.Fatalf("vm-image-server must have 2 images (base + uds-core), got %d", len(server.Images))
	}

	if len(server.Manifests) != 1 || !contains(server.Manifests[0].Files, "packages/vm-images/manifests/image-server.yaml") {
		t.Fatal("vm-image-server component must deploy packages/vm-images/manifests/image-server.yaml")
	}
	if len(imports.Manifests) != 1 || !contains(imports.Manifests[0].Files, "packages/vm-images/manifests/golden-pvcs.yaml") {
		t.Fatal("golden-pvcs component must deploy packages/vm-images/manifests/golden-pvcs.yaml")
	}
	if len(imports.Actions.OnDeploy.After) != 3 {
		t.Fatal("golden-pvcs component must wait for both imports before scaling down its servers")
	}
	for index, name := range []string{"golden-base", "golden-uds-core"} {
		wait := imports.Actions.OnDeploy.After[index].Wait.Cluster
		if wait.Kind != "datavolume" || wait.Name != name || wait.Namespace != "uds-lab-vms" || wait.Condition != "ready" {
			t.Fatalf("unexpected golden DataVolume readiness wait: %#v", wait)
		}
	}
	wantScale := "./zarf tools kubectl scale deployment/vm-image-base deployment/vm-image-uds-core --replicas=0 --namespace=uds-lab-vms"
	if got := imports.Actions.OnDeploy.After[2].Cmd; got != wantScale {
		t.Fatalf("image-server scale-down command = %q, want %q", got, wantScale)
	}
	if len(server.Actions.OnDeploy.After) != 2 {
		t.Fatal("vm-image-server component must wait for UDS networking and its server pods")
	}
	packageWait := server.Actions.OnDeploy.After[0].Wait.Cluster
	if packageWait.Kind != "package" || packageWait.Name != "lab-vm-images" || packageWait.Namespace != "uds-lab-vms" || packageWait.Condition != "ready" {
		t.Fatalf("unexpected UDS Package readiness wait: %#v", packageWait)
	}
	podWait := server.Actions.OnDeploy.After[1].Wait.Cluster
	if podWait.Kind != "pod" || podWait.Name != "app.kubernetes.io/part-of=lab-vm-images" || podWait.Namespace != "uds-lab-vms" || podWait.Condition != "ready" {
		t.Fatalf("unexpected image-server Pod readiness wait: %#v", podWait)
	}

	serverManifest, err := os.ReadFile("packages/vm-images/manifests/image-server.yaml")
	if err != nil {
		t.Fatal(err)
	}
	serverText := string(serverManifest)
	var udsPackage udsPackageManifest
	if err := yaml.Unmarshal(serverManifest, &udsPackage); err != nil {
		t.Fatalf("parse image-server UDS Package: %v", err)
	}
	wantAllows := []udsNetworkAllow{
		{
			Direction:       "Egress",
			Selector:        map[string]string{"cdi.kubevirt.io": "importer"},
			RemoteNamespace: "uds-lab-vms",
			RemoteSelector:  map[string]string{"app.kubernetes.io/part-of": "lab-vm-images"},
			Port:            8080,
		},
		{
			Direction:       "Ingress",
			Selector:        map[string]string{"app.kubernetes.io/part-of": "lab-vm-images"},
			RemoteNamespace: "uds-lab-vms",
			RemoteSelector:  map[string]string{"cdi.kubevirt.io": "importer"},
			Port:            8080,
		},
		{
			Direction:       "Egress",
			Selector:        map[string]string{"cdi.kubevirt.io": "cdi-clone-source"},
			RemoteNamespace: "uds-lab-vms",
			RemoteSelector:  map[string]string{"cdi.kubevirt.io": "cdi-upload-server"},
			Port:            8443,
		},
		{
			Direction:       "Ingress",
			Selector:        map[string]string{"cdi.kubevirt.io": "cdi-upload-server"},
			RemoteNamespace: "uds-lab-vms",
			RemoteSelector:  map[string]string{"cdi.kubevirt.io": "cdi-clone-source"},
			Port:            8443,
		},
	}
	if !reflect.DeepEqual(udsPackage.Spec.Network.Allow, wantAllows) {
		t.Fatalf("image-server UDS network policy is not least-privilege:\n got: %#v\nwant: %#v", udsPackage.Spec.Network.Allow, wantAllows)
	}
	for _, image := range []string{
		"ghcr.io/enxoco/lab-vm-images/base:###ZARF_CONST_VM_IMAGE_TAG###",
		"ghcr.io/enxoco/lab-vm-images/uds-core:###ZARF_CONST_VM_IMAGE_TAG###",
	} {
		if !strings.Contains(serverText, image) {
			t.Fatalf("image server manifest does not reference packaged image %q", image)
		}
	}

	dataVolumes, err := os.ReadFile("packages/vm-images/manifests/golden-pvcs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dvText := string(dataVolumes)
	for _, forbidden := range []string{"source:\n    registry:", "secretRef:", "ZARF_REGISTRY", "CDI_REGISTRY", "zarf-docker-registry"} {
		if strings.Contains(dvText, forbidden) {
			t.Fatalf("DataVolumes must not contain registry wiring %q", forbidden)
		}
	}
	for _, url := range []string{
		"http://vm-image-base.uds-lab-vms.svc.cluster.local:8080/lab-base.qcow2",
		"http://vm-image-uds-core.uds-lab-vms.svc.cluster.local:8080/lab-playground-uds-core.qcow2",
	} {
		if !strings.Contains(dvText, url) {
			t.Fatalf("DataVolume manifest does not contain image-server URL %q", url)
		}
	}

	for _, dockerfile := range []string{
		"packages/vm-images/Dockerfile.base",
		"packages/vm-images/Dockerfile.uds-core",
	} {
		contents, err := os.ReadFile(dockerfile)
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		for _, required := range []string{"FROM python:", "USER 65532:65532", `CMD ["python3", "-m", "http.server"`} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s does not contain %q", dockerfile, required)
			}
		}
		if strings.Contains(text, "FROM scratch") {
			t.Fatalf("%s cannot serve HTTP from a scratch image", dockerfile)
		}
	}
}

func TestPackerDownloadsRetryConnectionFailures(t *testing.T) {
	contents, err := os.ReadFile("packer/scripts/base.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(contents)
	for _, required := range []string{
		"curl_retry()",
		"--retry-all-errors",
		"--connect-timeout",
		"--max-time",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("packer download helper does not contain %q", required)
		}
	}
	if count := strings.Count(script, "curl -fsSL"); count != 1 {
		t.Fatalf("all packer downloads must use curl_retry; found %d direct curl invocations", count)
	}
}

func TestApplicationImageAndVersionsStayConsistent(t *testing.T) {
	var pkg applicationPackage
	readYAML(t, "zarf.yaml", &pkg)

	var values chartValues
	readYAML(t, "chart/values.yaml", &values)

	var chart chartMetadata
	readYAML(t, "chart/Chart.yaml", &chart)

	for _, variable := range pkg.Variables {
		if variable.Name == "IMAGE" {
			t.Fatal("image must not be a deployment variable because an override can name an image absent from the package")
		}
	}

	for _, component := range pkg.Components {
		if component.Name != "lab-platform" {
			continue
		}
		if len(component.Images) != 1 {
			t.Fatalf("lab-platform component must package exactly one application image, got %d", len(component.Images))
		}
		if component.Images[0] != values.Image {
			t.Fatalf("Zarf image %q differs from chart image %q", component.Images[0], values.Image)
		}
		if !strings.HasPrefix(values.Image, "ghcr.io/enxoco/") {
			t.Fatalf("application image must use ghcr.io/enxoco registry, got %q", values.Image)
		}
		if strings.HasSuffix(values.Image, ":latest") {
			t.Fatal("application image must use an immutable version tag")
		}
		if !strings.HasSuffix(values.Image, ":"+pkg.Metadata.Version) {
			t.Fatalf("application image %q must be tagged with package version %q", values.Image, pkg.Metadata.Version)
		}
		if chart.Version != pkg.Metadata.Version || chart.AppVersion != pkg.Metadata.Version {
			t.Fatalf("package version %q, chart version %q, and appVersion %q must match", pkg.Metadata.Version, chart.Version, chart.AppVersion)
		}
		if len(component.Charts) != 1 || component.Charts[0].Version != chart.Version {
			t.Fatal("Zarf chart version must match chart/Chart.yaml")
		}
		return
	}

	t.Fatal("application package has no lab-platform component")
}

func readYAML(t *testing.T, path string, target any) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(contents, target); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
