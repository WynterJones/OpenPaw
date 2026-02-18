package updater

import "testing"

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		input   string
		want    SemVer
		wantErr bool
	}{
		{"0.1.0", SemVer{0, 1, 0}, false},
		{"v0.1.0", SemVer{0, 1, 0}, false},
		{"1.2.3", SemVer{1, 2, 3}, false},
		{"v10.20.30", SemVer{10, 20, 30}, false},
		{"", SemVer{}, true},
		{"1.2", SemVer{}, true},
		{"v1.2", SemVer{}, true},
		{"abc", SemVer{}, true},
		{"1.2.x", SemVer{}, true},
		{"1.2.3.4", SemVer{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSemVer(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSemVer(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseSemVer(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSemVerIsNewer(t *testing.T) {
	tests := []struct {
		name string
		v    SemVer
		other SemVer
		want bool
	}{
		{"major newer", SemVer{2, 0, 0}, SemVer{1, 0, 0}, true},
		{"major older", SemVer{1, 0, 0}, SemVer{2, 0, 0}, false},
		{"minor newer", SemVer{1, 2, 0}, SemVer{1, 1, 0}, true},
		{"minor older", SemVer{1, 1, 0}, SemVer{1, 2, 0}, false},
		{"patch newer", SemVer{1, 1, 2}, SemVer{1, 1, 1}, true},
		{"patch older", SemVer{1, 1, 1}, SemVer{1, 1, 2}, false},
		{"equal", SemVer{1, 1, 1}, SemVer{1, 1, 1}, false},
		{"zero versions", SemVer{0, 0, 0}, SemVer{0, 0, 0}, false},
		{"zero to one", SemVer{0, 1, 0}, SemVer{0, 0, 1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.IsNewer(tt.other); got != tt.want {
				t.Errorf("%v.IsNewer(%v) = %v, want %v", tt.v, tt.other, got, tt.want)
			}
		})
	}
}

func TestSemVerString(t *testing.T) {
	v := SemVer{1, 2, 3}
	if s := v.String(); s != "1.2.3" {
		t.Errorf("String() = %q, want %q", s, "1.2.3")
	}
}
