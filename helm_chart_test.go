package labplatform_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func helmTemplate(t *testing.T, extraArgs ...string) string {
	t.Helper()
	args := append([]string{"template", "lab-platform", "./chart"}, extraArgs...)
	out, err := exec.Command("helm", args...).Output()
	if err != nil {
		t.Fatalf("helm template: %v\nstderr: %s", err, extractStderr(err))
	}
	return string(out)
}

func helmTemplateCommand(extraArgs ...string) *exec.Cmd {
	args := append([]string{"template", "lab-platform", "./chart"}, extraArgs...)
	return exec.Command("helm", args...)
}

func extractStderr(err error) []byte {
	var ee *exec.ExitError
	if exitErr, ok := err.(*exec.ExitError); ok {
		ee = exitErr
	}
	if ee != nil {
		return ee.Stderr
	}
	return nil
}

func TestHelmChart_OperatorConfigMapRendered(t *testing.T) {
	out := helmTemplate(t,
		"--set", "goldenPVCs.base=golden-base",
		"--set", "goldenPVCs.uds-core=golden-uds-core",
	)
	if !strings.Contains(out, "kind: ConfigMap") {
		t.Error("helm output missing ConfigMap")
	}
	if !strings.Contains(out, "golden-base") {
		t.Error("helm output missing golden-base PVC name")
	}
	if !strings.Contains(out, "golden-uds-core") {
		t.Error("helm output missing golden-uds-core PVC name")
	}
}

func TestHelmChart_OperatorConfigMapHasGoldenPVCNamespace(t *testing.T) {
	out := helmTemplate(t,
		"--set", "goldenPVCNamespace=uds-lab-golden",
		"--set", "goldenPVCs.base=golden-base",
	)
	if !strings.Contains(out, "uds-lab-golden") {
		t.Error("helm output missing goldenPVCNamespace")
	}
}

func TestHelmChart_OperatorDeploymentHasOperatorConfigEnv(t *testing.T) {
	out := helmTemplate(t)
	if !strings.Contains(out, "OPERATOR_CONFIG") {
		t.Error("operator deployment missing OPERATOR_CONFIG env var")
	}
}

func TestHelmChart_OperatorDeploymentHasConfigMapVolumeMount(t *testing.T) {
	out := helmTemplate(t)
	if !bytes.Contains([]byte(out), []byte("lab-operator-config")) {
		t.Error("operator deployment missing lab-operator-config volume/mount")
	}
}

func TestHelmChart_OperatorRBACCoversVMI(t *testing.T) {
	out := helmTemplate(t)
	if !strings.Contains(out, "virtualmachineinstances") {
		t.Error("operator Role missing virtualmachineinstances permission")
	}
}

func TestHelmChart_OperatorRBACCoversDataVolumes(t *testing.T) {
	out := helmTemplate(t)
	if !strings.Contains(out, "datavolumes") {
		t.Error("operator Role missing datavolumes permission")
	}
}

func TestHelmChart_OperatorRBACCoversLabSessionStatus(t *testing.T) {
	out := helmTemplate(t)
	if !strings.Contains(out, "labsessions/status") {
		t.Error("operator Role missing labsessions/status permission")
	}
}

func TestHelmChart_UsesPackagedImageByDefault(t *testing.T) {
	out := helmTemplate(t)
	var values chartValues
	readYAML(t, "chart/values.yaml", &values)

	if got := strings.Count(out, "image: "+values.Image); got != 2 {
		t.Fatalf("expected packaged image in both Deployments, got %d occurrences", got)
	}
	if strings.Contains(out, ":latest") {
		t.Fatal("rendered chart must not use a mutable latest tag")
	}
	if got := strings.Count(out, "imagePullPolicy: Always"); got != 2 {
		t.Fatalf("expected Always in both Deployments so same-version Zarf redeploys pull rebuilt images, got %d occurrences", got)
	}
}

func TestHelmChart_PodProviderOmitsKubeVirtResources(t *testing.T) {
	out := helmTemplate(t, "--set", "provider=pod")
	for _, resource := range []string{"datavolumes", "virtualmachineinstances", "volumesnapshots"} {
		if strings.Contains(out, resource) {
			t.Errorf("pod provider unexpectedly rendered %s RBAC", resource)
		}
	}
}

func TestHelmChart_RejectsUnsupportedProvider(t *testing.T) {
	cmd := helmTemplateCommand("--set", "provider=invalid")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected invalid provider to fail Helm validation\n%s", out)
	}
	if !strings.Contains(string(out), "provider") {
		t.Fatalf("expected provider validation error, got:\n%s", out)
	}
}
