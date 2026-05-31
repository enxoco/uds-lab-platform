package sizing

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      Size
		want    Size
		wantErr bool
	}{
		{"empty defaults to medium", "", Default, false},
		{"small", Small, Small, false},
		{"medium", Medium, Medium, false},
		{"large", Large, Large, false},
		{"unknown is rejected", "xlarge", "", true},
		{"case sensitive", "Large", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Normalize(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDefaultsCoverEveryTier(t *testing.T) {
	for _, s := range []Size{Small, Medium, Large} {
		spec, ok := Defaults[s]
		if !ok {
			t.Errorf("Defaults missing tier %q", s)
			continue
		}
		if spec.CPU == "" || spec.Memory == "" {
			t.Errorf("Defaults[%q] has empty field: %+v", s, spec)
		}
	}
	if !Valid(Default) {
		t.Errorf("Default tier %q is not Valid", Default)
	}
}
