// Package sizing defines the abstract, provider-agnostic VM size tiers used by
// scenarios (ADR-0013). A scenario declares an abstract size (small|medium|large);
// the operator maps that tier to provider-specific resources (KubeVirt
// resources.requests, Azure VMSS SKU, etc.) — the mapping is intentionally NOT
// here, so this package carries no Kubernetes dependency and can be imported by
// the thin platform server.
package sizing

import "fmt"

// Size is an abstract T-shirt size tier.
type Size string

const (
	Small  Size = "small"
	Medium Size = "medium"
	Large  Size = "large"
)

// Default is applied when a scenario omits a size.
const Default = Medium

// Spec is the provider-agnostic resource shape for a size tier. The operator
// translates this (or its ConfigMap override) into provider-specific values.
type Spec struct {
	CPU    string
	Memory string
}

// Defaults is the built-in size table (ADR-0013). The operator may override
// these via its ConfigMap; the platform server only needs the tier names.
var Defaults = map[Size]Spec{
	Small:  {CPU: "2", Memory: "4Gi"},
	Medium: {CPU: "4", Memory: "8Gi"},
	Large:  {CPU: "8", Memory: "16Gi"},
}

// Valid reports whether s is a known size tier.
func Valid(s Size) bool {
	_, ok := Defaults[s]
	return ok
}

// Normalize returns the size to use for a scenario: the declared size if valid,
// or Default when empty. A non-empty but unknown size is an error so scenario
// authors get a clear signal rather than a silent fallback.
func Normalize(s Size) (Size, error) {
	if s == "" {
		return Default, nil
	}
	if !Valid(s) {
		return "", fmt.Errorf("unknown size %q (want one of small, medium, large)", s)
	}
	return s, nil
}
