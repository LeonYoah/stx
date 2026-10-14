package auth

import "testing"

func TestNormalizeTimezone(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"":                 {"", true},
		"  Asia/Shanghai ": {"Asia/Shanghai", true},
		"UTC":              {"UTC", true},
		"Local":            {"", false},
		"Mars/Base":        {"", false},
	}
	for in, c := range cases {
		got, ok := NormalizeTimezone(in)
		if got != c.want || ok != c.ok {
			t.Fatalf("%q: got (%q,%v) want (%q,%v)", in, got, ok, c.want, c.ok)
		}
	}
}
