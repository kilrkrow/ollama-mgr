package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFetchedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Config{ConfigDir: dir, CacheDir: filepath.Join(dir, "cache")}
	if err := c.SaveFetchedBases([]string{"mistral", "phi3", "mistral", " "}); err != nil {
		t.Fatal(err)
	}
	got := c.LoadFetchedBases()
	if len(got) != 2 || got[0] != "mistral" || got[1] != "phi3" {
		t.Fatalf("got %v", got)
	}
	// ensure file exists
	if _, err := os.Stat(c.FetchedPath()); err != nil {
		t.Fatal(err)
	}
}
