package family

import (
	"context"
	"testing"
	"time"

	"github.com/kilrkrow/ollama-mgr/internal/catalog"
	"github.com/kilrkrow/ollama-mgr/internal/ollama"
)

type fakeEnrich struct {
	pills map[string]catalog.FamilyPills
}

func (f fakeEnrich) FamilyPills(ctx context.Context, base string) (catalog.FamilyPills, error) {
	if p, ok := f.pills[base]; ok {
		return p, nil
	}
	return catalog.FamilyPills{}, nil
}

func TestFetchLibraryFamilyEmptySizesOutline(t *testing.T) {
	enrich := fakeEnrich{pills: map[string]catalog.FamilyPills{
		"mistral": {Name: "mistral", Features: []string{"tools"}, Sizes: []string{"7b", "small"}},
	}}
	// CanonicalSizePills may drop "small" â€” use pure sizes
	enrich.pills["mistral"] = catalog.FamilyPills{Name: "mistral", Features: []string{"tools"}, Sizes: []string{"7b", "22b"}}
	f, err := FetchLibraryFamily(context.Background(), "mistral", enrich)
	if err != nil {
		t.Fatal(err)
	}
	if f.OnDisk || !f.Fetched {
		t.Fatalf("expected fetched empty: %+v", f)
	}
	if len(f.Sizes) < 1 {
		t.Fatal("expected size pills")
	}
	for _, s := range f.Sizes {
		if s.Installed {
			t.Fatalf("all should be outline: %+v", s)
		}
		if !s.Available {
			t.Fatalf("should be available: %+v", s)
		}
	}
}

func TestGroupWithFetched(t *testing.T) {
	enrich := fakeEnrich{pills: map[string]catalog.FamilyPills{
		"mistral": {Name: "mistral", Features: []string{"tools"}, Sizes: []string{"7b"}},
	}}
	fams := GroupWithFetched(context.Background(), nil, []string{"mistral"}, enrich)
	if len(fams) != 1 || fams[0].Base != "mistral" || !fams[0].Fetched {
		t.Fatalf("%+v", fams)
	}
}

func TestGroupSizePillsInstalledVsAvailable(t *testing.T) {
	models := []ollama.Model{
		{
			Name: "qwen3-coder:30b", Size: 18e9, ParameterSize: "30.5B", QuantizationLevel: "Q4_K_M",
			Capabilities: []string{"completion", "tools"},
			ModifiedAt:   time.Now(),
		},
	}
	enrich := fakeEnrich{pills: map[string]catalog.FamilyPills{
		"qwen3-coder": {Name: "qwen3-coder", Features: []string{"tools"}, Sizes: []string{"30b", "480b"}},
	}}
	fams := Group(context.Background(), models, enrich)
	if len(fams) != 1 {
		t.Fatalf("families=%d", len(fams))
	}
	f := fams[0]
	if f.Base != "qwen3-coder" {
		t.Fatalf("base=%s", f.Base)
	}
	var got30, got480 *SizePill
	for i := range f.Sizes {
		if f.Sizes[i].Size == "30b" {
			got30 = &f.Sizes[i]
		}
		if f.Sizes[i].Size == "480b" {
			got480 = &f.Sizes[i]
		}
	}
	if got30 == nil || !got30.Installed {
		t.Fatalf("30b should be installed: %+v", got30)
	}
	if got480 == nil || got480.Installed || !got480.Available {
		t.Fatalf("480b should be available not installed: %+v", got480)
	}
	hasTools := false
	for _, feat := range f.Features {
		if feat.Name == "tools" && feat.Local {
			hasTools = true
		}
	}
	if !hasTools {
		t.Fatalf("expected tools feature: %+v", f.Features)
	}
}
