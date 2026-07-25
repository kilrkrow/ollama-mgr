package modelparse

import (
	"testing"
)

func TestParseQwenCoder(t *testing.T) {
	p := Parse("qwen2.5-coder:32b", "32.0B")
	if p.Family != "qwen" {
		t.Fatalf("family=%q want qwen", p.Family)
	}
	if p.Specialty != "coder" {
		t.Fatalf("specialty=%q want coder", p.Specialty)
	}
	if p.SizeClass != "32b" {
		t.Fatalf("size=%q want 32b", p.SizeClass)
	}
	if p.Version.String() != "2.5" && p.Version.Raw != "2.5" {
		// accept 2.5 from parser
		if len(p.Version.Parts) < 2 || p.Version.Parts[0] != 2 || p.Version.Parts[1] != 5 {
			t.Fatalf("version=%v raw=%q", p.Version.Parts, p.Version.Raw)
		}
	}
	if p.LibraryURL() != "https://ollama.com/library/qwen2.5-coder:32b" {
		t.Fatalf("url=%s", p.LibraryURL())
	}
}

func TestVersionCompare(t *testing.T) {
	a := ParseVersion("2.5")
	b := ParseVersion("3")
	c := ParseVersion("3.5")
	if a.Compare(b) >= 0 {
		t.Fatal("2.5 should be < 3")
	}
	if b.Compare(c) >= 0 {
		t.Fatal("3 should be < 3.5")
	}
	if c.Compare(a) <= 0 {
		t.Fatal("3.5 should be > 2.5")
	}
}

func TestSizeCompatible(t *testing.T) {
	if !SizeCompatible("32b", "32b") {
		t.Fatal("exact")
	}
	if !SizeCompatible("32b", "33b") {
		t.Fatal("near 33")
	}
	if !SizeCompatible("32b", "30b") {
		t.Fatal("near 30 (qwen2.5 32b â†’ qwen3 30b)")
	}
	if SizeCompatible("7b", "32b") {
		t.Fatal("different")
	}
	if SizeCompatible("7b", "14b") {
		t.Fatal("7 vs 14 should not match")
	}
}

func TestParameterSizeFallback(t *testing.T) {
	p := Parse("mistral:latest", "7.2B")
	if p.SizeClass != "7.2b" && p.SizeClass != "7b" {
		// 7.2B normalizes to 7.2b
		if p.SizeClass != "7.2b" {
			t.Fatalf("size=%q", p.SizeClass)
		}
	}
}
