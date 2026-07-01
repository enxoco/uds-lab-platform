// Package v1alpha1 defines the LabSession custom resource (ADR-0011).
//
// LabSession is the lifecycle handle the thin platform server creates and the
// Lab Operator reconciles into a provider VM (KubeVirt VMI today) plus its
// Service and NetworkPolicy. All session metadata lives on the spec so that
// operator state is recoverable by listing — no in-memory session map.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the group/version for the LabSession API.
var GroupVersion = schema.GroupVersion{Group: "lab.uds.dev", Version: "v1alpha1"}

// SchemeBuilder registers the LabSession types with a runtime scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme adds the LabSession types to a scheme.
var AddToScheme = SchemeBuilder.AddToScheme

func init() {
	SchemeBuilder.Register(&LabSession{}, &LabSessionList{})
}
