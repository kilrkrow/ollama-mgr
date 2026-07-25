package catalog

import "testing"

func TestParseUpdatedTitle(t *testing.T) {
	html := `<span class="flex items-center" title="May 28, 2025 1:19 AM UTC">
                      <span class="hidden sm:flex">Updated&nbsp;</span>
                      <span >1 year ago</span>
                    </span>`
	raw, ts, ok := parseUpdatedTitle(html)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if raw != "May 28, 2025 1:19 AM UTC" {
		t.Fatalf("raw=%q", raw)
	}
	if ts.Year() != 2025 || ts.Month() != 5 || ts.Day() != 28 {
		t.Fatalf("time=%v", ts)
	}
}
