package origin

import "testing"

func TestLookupKnown(t *testing.T) {
	cases := map[string]string{
		"qwen3-coder": "CN",
		"qwen2.5-coder": "CN",
		"llama3.1":    "US",
		"gemma4":      "US",
		"mistral":     "FR",
		"deepseek-r1": "CN",
		"phi3":        "US",
		"nemotron3":   "US",
		"command-r":   "CA",
	}
	for base, code := range cases {
		info := Lookup(base)
		if info.Unknown || info.Code != code {
			t.Fatalf("%s: got %+v want %s", base, info, code)
		}
		if info.Flag == "" || info.Flag == "🏳️" {
			t.Fatalf("%s: missing flag emoji", base)
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	info := Lookup("totally-made-up-xyz-model")
	if !info.Unknown {
		t.Fatalf("expected unknown: %+v", info)
	}
}
