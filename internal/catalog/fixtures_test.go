package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFixturePopular(t *testing.T) {
	html := loadFixture(t, "sample_library.html")
	list := parsePopularHTML(html)
	if len(list) < 2 {
		t.Fatalf("got %d", len(list))
	}
	if list[0].Name != "llama3.1" {
		t.Fatalf("%+v", list[0])
	}
	if list[0].Pulls == "" {
		t.Log("pulls optional in fixture")
	}
}

func TestFixtureFamilyPills(t *testing.T) {
	html := loadFixture(t, "sample_library.html")
	features := uniqueLower(featurePillRe.FindAllStringSubmatch(html, -1))
	sizes := uniqueLower(sizePillRe.FindAllStringSubmatch(html, -1))
	if len(features) == 0 || features[0] != "tools" {
		t.Fatalf("features=%v", features)
	}
	canon := CanonicalSizePills(sizes)
	if len(canon) < 2 {
		t.Fatalf("sizes=%v canon=%v", sizes, canon)
	}
}

func TestFixtureUpdatedTitle(t *testing.T) {
	html := loadFixture(t, "sample_library.html")
	raw, ts, ok := parseUpdatedTitle(html)
	if !ok {
		t.Fatal("expected updated title")
	}
	if ts.Year() != 2025 {
		t.Fatalf("raw=%s ts=%v", raw, ts)
	}
}

func TestFixtureTags(t *testing.T) {
	html := loadFixture(t, "sample_library.html")
	tags := parseTagsHTML(html, "mistral")
	if len(tags) < 2 {
		t.Fatalf("tags=%v", tags)
	}
}
