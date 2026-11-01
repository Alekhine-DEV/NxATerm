package appver

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.1.0", "1.0.0", true},
		{"v1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.2.0", false},
		{"2.0", "1.9.9", true},
		{"1.10.0", "1.9.0", true},
		{"1.0.0", "dev", false},
		{"dev", "1.0.0", false},
		{"1.1.0-rc1", "1.0.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q,%q)=%v", c.a, c.b, got)
		}
	}
}
