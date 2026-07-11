// Command laboperator is the Lab Operator (ADR-0011): it watches LabSession
// objects and reconciles each into a provider VM (KubeVirt VMI), Service, and
// NetworkPolicy, enforces TTL, and reports readiness. It holds no in-memory
// session state — restarts recover by listing.
package main

import (
	"io/fs"
	"os"
	"text/template" // nosemgrep: go.lang.security.audit.xss.import-text-template.import-text-template -- renders shell scripts, not HTML

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	kvv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	labplatform "github.com/enxoco/uds-lab-platform"
	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
	"github.com/enxoco/uds-lab-platform/internal/controller"
	"github.com/enxoco/uds-lab-platform/internal/operator"
	"github.com/enxoco/uds-lab-platform/internal/provider/kubevirt"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(labv1.AddToScheme(scheme))
	utilruntime.Must(kvv1.AddToScheme(scheme))
	utilruntime.Must(cdiv1.AddToScheme(scheme))
	utilruntime.Must(snapshotv1.AddToScheme(scheme))
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	setupLog := ctrl.Log.WithName("setup")

	vmNamespace := envOr("VM_NAMESPACE", "uds-lab-vms")
	serverNamespace := envOr("SERVER_NAMESPACE", "uds-lab-platform")
	storageClass := os.Getenv("STORAGE_CLASS")

	// Operator ConfigMap: size tiers + image refs (ADR-0013).
	cfg, err := operator.Load(os.Getenv("OPERATOR_CONFIG"))
	if err != nil {
		setupLog.Error(err, "load operator config")
		os.Exit(1)
	}
	sizeOverrides, err := cfg.SizeOverrides()
	if err != nil {
		setupLog.Error(err, "operator config sizes")
		os.Exit(1)
	}

	// Scenario content + cloud-init template, embedded (Decision D1).
	scenariosFS, err := fs.Sub(labplatform.ScenariosFS, "scenarios")
	if err != nil {
		setupLog.Error(err, "embedded scenarios")
		os.Exit(1)
	}
	vmFS, err := fs.Sub(labplatform.VMFiles, "vm")
	if err != nil {
		setupLog.Error(err, "embedded vm")
		os.Exit(1)
	}
	tmplData, err := fs.ReadFile(vmFS, "user-data.sh.gotmpl")
	if err != nil {
		setupLog.Error(err, "load user-data template")
		os.Exit(1)
	}
	udTmpl, err := template.New("user-data.sh.gotmpl").Parse(string(tmplData))
	if err != nil {
		setupLog.Error(err, "parse user-data template")
		os.Exit(1)
	}
	injectPy, err := fs.ReadFile(vmFS, "lab-inject.py")
	if err != nil {
		setupLog.Error(err, "load lab-inject.py")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "create manager")
		os.Exit(1)
	}

	prov := kubevirt.New(kubevirt.Config{
		Client:          mgr.GetClient(),
		Namespace:       vmNamespace,
		ServerNamespace: serverNamespace,
		UserDataTmpl:    udTmpl,
		ScenariosFS:     scenariosFS,
		InjectPy:        string(injectPy),
		SizeOverrides:   sizeOverrides,
		GoldenPVCs:         cfg.GoldenPVCs,
		GoldenPVCNamespace: cfg.GoldenPVCNamespace,
		GoldenPVCDiskSize:  cfg.GoldenPVCDiskSize,
		StorageClass:    storageClass,
	})

	if err := (&controller.LabSessionReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Provider: prov,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "setup reconciler")
		os.Exit(1)
	}

	setupLog.Info("starting lab operator", "vmNamespace", vmNamespace, "serverNamespace", serverNamespace)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "manager exited")
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
