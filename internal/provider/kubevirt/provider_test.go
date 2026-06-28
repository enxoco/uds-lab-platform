package kubevirt

import (
	"context"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	labv1 "github.com/defenseunicorns/uds-lab-platform/api/v1alpha1"
)

// scenariosFS points at the real scenario fixtures relative to this package.
var scenariosFS = os.DirFS("../../../scenarios")

// ── goldenPVCForScenario ──────────────────────────────────────────────────────

func TestGoldenPVCForScenario_BaseFallback(t *testing.T) {
	p := New(Config{
		ScenariosFS: scenariosFS,
		GoldenPVCs:  map[string]string{"base": "golden-base"},
	})
	got, err := p.goldenPVCForScenario("uds-package-quickstart")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "golden-base" {
		t.Errorf("PVC name = %q, want golden-base", got)
	}
}

func TestGoldenPVCForScenario_PlaygroundToolsTier(t *testing.T) {
	p := New(Config{
		ScenariosFS: scenariosFS,
		GoldenPVCs:  map[string]string{"tools": "golden-tools"},
	})
	got, err := p.goldenPVCForScenario("playground-tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "golden-tools" {
		t.Errorf("PVC name = %q, want golden-tools", got)
	}
}

func TestGoldenPVCForScenario_PlaygroundUDSCoreTier(t *testing.T) {
	p := New(Config{
		ScenariosFS: scenariosFS,
		GoldenPVCs:  map[string]string{"uds-core": "golden-uds-core"},
	})
	got, err := p.goldenPVCForScenario("playground-uds-core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "golden-uds-core" {
		t.Errorf("PVC name = %q, want golden-uds-core", got)
	}
}

func TestGoldenPVCForScenario_MissingTierReturnsError(t *testing.T) {
	p := New(Config{
		ScenariosFS: scenariosFS,
		GoldenPVCs:  map[string]string{},
	})
	_, err := p.goldenPVCForScenario("uds-package-quickstart")
	if err == nil {
		t.Fatal("expected error for unconfigured tier")
	}
}

func TestGoldenPVCForScenario_UnknownScenarioReturnsError(t *testing.T) {
	p := New(Config{
		ScenariosFS: scenariosFS,
		GoldenPVCs:  map[string]string{"base": "golden-base"},
	})
	_, err := p.goldenPVCForScenario("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown scenario")
	}
}

// ── ensureDataVolume ──────────────────────────────────────────────────────────

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("add clientgo scheme: %v", err)
	}
	if err := cdiv1.AddToScheme(s); err != nil {
		t.Fatalf("add cdi scheme: %v", err)
	}
	if err := labv1.AddToScheme(s); err != nil {
		t.Fatalf("add labv1 scheme: %v", err)
	}
	return s
}

func testLabSession(name, namespace string) *labv1.LabSession {
	return &labv1.LabSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-uid-" + name),
		},
	}
}

func TestEnsureDataVolume_UsesPVCCloneSource(t *testing.T) {
	s := testScheme(t)
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	ls := testLabSession("test-ls", "uds-lab-vms")

	p := New(Config{
		Client:             fc,
		Namespace:          "uds-lab-vms",
		GoldenPVCs:         map[string]string{"base": "golden-base"},
		GoldenPVCNamespace: "uds-lab-golden",
		GoldenPVCDiskSize:  "80Gi",
	})

	err := p.ensureDataVolume(context.Background(), ls, "lab-testdata", map[string]string{"test": "true"}, "golden-base")
	if err != nil {
		t.Fatalf("ensureDataVolume: %v", err)
	}

	dv := &cdiv1.DataVolume{}
	if err := fc.Get(context.Background(), client.ObjectKey{Namespace: "uds-lab-vms", Name: "lab-testdata"}, dv); err != nil {
		t.Fatalf("get DataVolume: %v", err)
	}

	if dv.Spec.Source == nil || dv.Spec.Source.PVC == nil {
		t.Fatal("expected PVC clone source, got nil")
	}
	if dv.Spec.Source.Registry != nil {
		t.Error("registry source must be nil for PVC clone")
	}
	if dv.Spec.Source.PVC.Namespace != "uds-lab-golden" {
		t.Errorf("source PVC namespace = %q, want uds-lab-golden", dv.Spec.Source.PVC.Namespace)
	}
	if dv.Spec.Source.PVC.Name != "golden-base" {
		t.Errorf("source PVC name = %q, want golden-base", dv.Spec.Source.PVC.Name)
	}
}

func TestEnsureDataVolume_NamespaceFallsBackToVMNamespace(t *testing.T) {
	s := testScheme(t)
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	ls := testLabSession("test-ls2", "uds-lab-vms")

	p := New(Config{
		Client:             fc,
		Namespace:          "uds-lab-vms",
		GoldenPVCs:         map[string]string{"base": "golden-base"},
		GoldenPVCNamespace: "", // empty → fall back to Namespace
		GoldenPVCDiskSize:  "80Gi",
	})

	if err := p.ensureDataVolume(context.Background(), ls, "lab-testdata2", nil, "golden-base"); err != nil {
		t.Fatalf("ensureDataVolume: %v", err)
	}

	dv := &cdiv1.DataVolume{}
	if err := fc.Get(context.Background(), client.ObjectKey{Namespace: "uds-lab-vms", Name: "lab-testdata2"}, dv); err != nil {
		t.Fatalf("get DataVolume: %v", err)
	}

	if dv.Spec.Source.PVC.Namespace != "uds-lab-vms" {
		t.Errorf("source PVC namespace = %q, want uds-lab-vms (fallback)", dv.Spec.Source.PVC.Namespace)
	}
}

func TestEnsureDataVolume_DiskSizeFallsBackToDefault(t *testing.T) {
	s := testScheme(t)
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	ls := testLabSession("test-ls3", "uds-lab-vms")

	p := New(Config{
		Client:            fc,
		Namespace:         "uds-lab-vms",
		GoldenPVCs:        map[string]string{"base": "golden-base"},
		GoldenPVCDiskSize: "", // empty → use defaultDiskSize
	})

	if err := p.ensureDataVolume(context.Background(), ls, "lab-testdata3", nil, "golden-base"); err != nil {
		t.Fatalf("ensureDataVolume: %v", err)
	}

	dv := &cdiv1.DataVolume{}
	if err := fc.Get(context.Background(), client.ObjectKey{Namespace: "uds-lab-vms", Name: "lab-testdata3"}, dv); err != nil {
		t.Fatalf("get DataVolume: %v", err)
	}

	storage := dv.Spec.PVC.Resources.Requests["storage"]
	if storage.String() != defaultDiskSize {
		t.Errorf("storage = %q, want %q", storage.String(), defaultDiskSize)
	}
}
