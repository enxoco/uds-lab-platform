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
