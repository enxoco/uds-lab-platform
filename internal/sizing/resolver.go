package sizing

// Resolve returns the resource Spec for a size tier, preferring an operator
// override (loaded from the operator's ConfigMap, ADR-0013) and falling back to
// the built-in Defaults. The overrides map is built by the operator from
// ConfigMap data and passed in here, so this package stays Kubernetes-free.
//
// The size is assumed already validated via Normalize; an unknown tier yields
// the zero Spec and ok=false.
func Resolve(s Size, overrides map[Size]Spec) (Spec, bool) {
	if spec, ok := overrides[s]; ok {
		return spec, true
	}
	spec, ok := Defaults[s]
	return spec, ok
}
