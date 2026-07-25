package config

import "testing"

func TestNormalizeEndpointBindAddresses(t *testing.T) {
	cases := map[string]string{
		"0.0.0.0:11434":          "http://127.0.0.1:11434",
		"http://0.0.0.0:11434":   "http://127.0.0.1:11434",
		"http://0.0.0.0":         "http://127.0.0.1",
		"http://[::]:11434":      "http://127.0.0.1:11434",
		"http://127.0.0.1:11434": "http://127.0.0.1:11434",
		"http://localhost:11434": "http://localhost:11434",
		"gpu.local:11434":        "http://gpu.local:11434",
		"":                       DefaultEndpoint,
	}
	for in, want := range cases {
		got := normalizeEndpoint(in)
		if got != want {
			t.Errorf("normalizeEndpoint(%q)=%q want %q", in, got, want)
		}
	}
}
