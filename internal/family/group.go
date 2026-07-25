package family

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/guysc/ollama-mgr/internal/catalog"
	"github.com/guysc/ollama-mgr/internal/modelparse"
	"github.com/guysc/ollama-mgr/internal/ollama"
	"github.com/guysc/ollama-mgr/internal/origin"
)

// SizePill is a parameter-size chip (installed or available).
type SizePill struct {
	Size       string   `json:"size"`                  // e.g. 30b
	Installed  bool     `json:"installed"`
	Available  bool     `json:"available"`             // known from library
	DiskBytes  int64    `json:"disk_bytes,omitempty"`  // sum of local tags for this size
	DiskHuman  string   `json:"disk_human,omitempty"`
	Quant      string   `json:"quant,omitempty"`       // primary local quant if any
	PullTag    string   `json:"pull_tag"`              // base:size for pull
	LocalTags  []string `json:"local_tags,omitempty"`  // exact installed names
	CloudOnly  bool     `json:"cloud_only,omitempty"`
}

// FeaturePill is a capability chip.
type FeaturePill struct {
	Name  string `json:"name"`  // tools, vision, ...
	Local bool   `json:"local"` // seen on at least one installed tag
	Lib   bool   `json:"lib"`   // advertised on library page
}

// InstalledTag is one local model under a family.
type InstalledTag struct {
	Name         string   `json:"name"`
	SizeClass    string   `json:"size_class"`
	SizeBytes    int64    `json:"size_bytes"`
	SizeHuman    string   `json:"size_human"`
	Params       string   `json:"params"`
	Quant        string   `json:"quant"`
	Capabilities []string `json:"capabilities,omitempty"`
	ContextLen   int64    `json:"context_length,omitempty"`
	Modified     string   `json:"modified,omitempty"`
	LibraryURL   string   `json:"library_url"`
}

// Family is a grouped model line (e.g. qwen3-coder).
type Family struct {
	Base         string         `json:"base"`
	LibraryURL   string         `json:"library_url"`
	Features     []FeaturePill  `json:"features"`
	Sizes        []SizePill     `json:"sizes"`
	Installed    []InstalledTag `json:"installed"`
	DiskBytes    int64          `json:"disk_bytes"`
	DiskHuman    string         `json:"disk_human"`
	TagCount     int            `json:"tag_count"`
	Capabilities []string       `json:"capabilities"` // union local
	// Origin is curated country-of-origin (lab HQ), not from Ollama API.
	Origin origin.Info `json:"origin"`
	// Fetched is true when this row was added via library fetch (+) and may have zero local tags.
	Fetched bool `json:"fetched,omitempty"`
	// OnDisk is true when at least one local tag exists for this base.
	OnDisk bool `json:"on_disk"`
}

// Enricher supplies library metadata for a base name.
type Enricher interface {
	FamilyPills(ctx context.Context, base string) (catalog.FamilyPills, error)
}

// Group builds families from local models. If enrich is non-nil, library pills are merged.
func Group(ctx context.Context, models []ollama.Model, enrich Enricher) []Family {
	return GroupWithFetched(ctx, models, nil, enrich)
}

// GroupWithFetched is like Group, but also includes library bases that may have no local tags
// (added via "+" family fetch). fetchedBases that already appear in models are merged normally
// and marked Fetched=false once on disk (still listed once).
func GroupWithFetched(ctx context.Context, models []ollama.Model, fetchedBases []string, enrich Enricher) []Family {
	byBase := map[string][]ollama.Model{}
	order := []string{}
	localSet := map[string]bool{}
	for _, m := range models {
		p := modelparse.Parse(m.Name, m.ParameterSize)
		base := p.BaseName
		if base == "" {
			base = m.Name
		}
		if _, ok := byBase[base]; !ok {
			order = append(order, base)
		}
		byBase[base] = append(byBase[base], m)
		localSet[base] = true
	}

	fetchedSet := map[string]bool{}
	for _, b := range fetchedBases {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		// normalize: no tag
		if i := strings.Index(b, ":"); i >= 0 {
			b = b[:i]
		}
		if strings.Contains(b, "/") {
			// user/model — keep as-is for now
		}
		fetchedSet[b] = true
		if !localSet[b] {
			if _, ok := byBase[b]; !ok {
				order = append(order, b)
				byBase[b] = nil
			}
		}
	}
	sort.Strings(order)

	// parallel library fetch
	type libRes struct {
		base  string
		pills catalog.FamilyPills
	}
	libCh := make(chan libRes, len(order))
	if enrich != nil {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		for _, base := range order {
			base := base
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				p, err := enrich.FamilyPills(ctx, base)
				if err != nil {
					libCh <- libRes{base: base}
					return
				}
				libCh <- libRes{base: base, pills: p}
			}()
		}
		go func() {
			wg.Wait()
			close(libCh)
		}()
	} else {
		close(libCh)
	}
	libMap := map[string]catalog.FamilyPills{}
	for r := range libCh {
		libMap[r.base] = r.pills
	}

	out := make([]Family, 0, len(order))
	for _, base := range order {
		ms := byBase[base]
		f := buildFamily(base, ms, libMap[base])
		f.OnDisk = f.TagCount > 0
		// Mark as fetched if user added via + and still not on disk (or always if in set and empty)
		if fetchedSet[base] && !f.OnDisk {
			f.Fetched = true
		}
		out = append(out, f)
	}
	return out
}

// FetchLibraryFamily loads library pills for base and returns a Family with no local tags
// (all size pills outline). Used by "+" when the model is not installed yet.
func FetchLibraryFamily(ctx context.Context, base string, enrich Enricher) (Family, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return Family{}, fmt.Errorf("empty model name")
	}
	if i := strings.Index(base, ":"); i >= 0 {
		base = base[:i]
	}
	var pills catalog.FamilyPills
	if enrich != nil {
		p, err := enrich.FamilyPills(ctx, base)
		if err != nil {
			return Family{}, err
		}
		pills = p
	}
	if len(pills.Sizes) == 0 && len(pills.Features) == 0 && pills.Name == "" {
		// still allow empty-ish if page had no pills — but require enrich success with name
		pills.Name = base
	}
	f := buildFamily(base, nil, pills)
	f.Fetched = true
	f.OnDisk = false
	if len(f.Sizes) == 0 {
		return f, fmt.Errorf("no size pills found for %q (not a known library model?)", base)
	}
	return f, nil
}

func buildFamily(base string, models []ollama.Model, lib catalog.FamilyPills) Family {
	f := Family{
		Base:       base,
		LibraryURL: "https://ollama.com/library/" + base,
		Installed:  make([]InstalledTag, 0, len(models)),
		OnDisk:     len(models) > 0,
		Origin:     origin.Lookup(base),
	}
	if models == nil {
		models = []ollama.Model{}
	}

	// size class -> local tags
	type sizeAcc struct {
		tags  []string
		bytes int64
		quant string
	}
	localSizes := map[string]*sizeAcc{}
	capSet := map[string]bool{}

	for _, m := range models {
		p := modelparse.Parse(m.Name, m.ParameterSize)
		sc := p.SizeClass
		if sc == "" {
			sc = "unknown"
		}
		acc := localSizes[sc]
		if acc == nil {
			acc = &sizeAcc{}
			localSizes[sc] = acc
		}
		acc.tags = append(acc.tags, m.Name)
		acc.bytes += m.Size
		if acc.quant == "" {
			acc.quant = m.QuantizationLevel
		}
		f.DiskBytes += m.Size
		for _, c := range m.Capabilities {
			c = strings.ToLower(strings.TrimSpace(c))
			if c != "" && c != "completion" { // completion is ubiquitous; still include later if only one
				capSet[c] = true
			}
			if c == "completion" {
				capSet[c] = true
			}
		}
		f.Installed = append(f.Installed, InstalledTag{
			Name:         m.Name,
			SizeClass:    sc,
			SizeBytes:    m.Size,
			SizeHuman:    ollama.FormatSize(m.Size),
			Params:       m.ParameterSize,
			Quant:        m.QuantizationLevel,
			Capabilities: m.Capabilities,
			ContextLen:   m.ContextLength,
			Modified:     m.ModifiedAt.Local().Format("2006-01-02"),
			LibraryURL:   p.LibraryURL(),
		})
	}
	f.TagCount = len(f.Installed)
	f.DiskHuman = ollama.FormatSize(f.DiskBytes)

	// capabilities sorted
	for c := range capSet {
		f.Capabilities = append(f.Capabilities, c)
	}
	sort.Strings(f.Capabilities)

	// feature pills: union local + library
	featOrder := []string{"tools", "vision", "thinking", "audio", "insert", "embedding", "completion"}
	libFeat := map[string]bool{}
	for _, x := range lib.Features {
		libFeat[strings.ToLower(x)] = true
	}
	seenFeat := map[string]bool{}
	addFeat := func(name string) {
		name = strings.ToLower(name)
		if name == "" || seenFeat[name] {
			return
		}
		// skip bare completion if we have richer caps (still show if only completion)
		seenFeat[name] = true
		f.Features = append(f.Features, FeaturePill{
			Name:  name,
			Local: capSet[name],
			Lib:   libFeat[name],
		})
	}
	for _, name := range featOrder {
		if capSet[name] || libFeat[name] {
			addFeat(name)
		}
	}
	for name := range capSet {
		addFeat(name)
	}
	for name := range libFeat {
		addFeat(name)
	}

	// size pills: library canonical + any installed size classes
	sizeOrder := []string{}
	seenSize := map[string]bool{}
	addSizeOrder := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || s == "unknown" || seenSize[s] {
			return
		}
		// skip cloud-only size tokens as primary? still include with flag
		seenSize[s] = true
		sizeOrder = append(sizeOrder, s)
	}
	for _, s := range lib.Sizes {
		addSizeOrder(s)
	}
	// installed sizes not in library list
	instKeys := make([]string, 0, len(localSizes))
	for sc := range localSizes {
		instKeys = append(instKeys, sc)
	}
	sort.Slice(instKeys, func(i, j int) bool {
		return sizeRank(instKeys[i]) < sizeRank(instKeys[j])
	})
	for _, sc := range instKeys {
		addSizeOrder(sc)
	}
	// stable-ish sort by numeric size
	sort.SliceStable(sizeOrder, func(i, j int) bool {
		return sizeRank(sizeOrder[i]) < sizeRank(sizeOrder[j])
	})

	libSizeSet := map[string]bool{}
	for _, s := range lib.Sizes {
		libSizeSet[strings.ToLower(s)] = true
	}

	for _, sc := range sizeOrder {
		if strings.Contains(sc, "cloud") {
			continue // hide cloud pills in v1
		}
		pill := SizePill{
			Size:      sc,
			Available: libSizeSet[sc] || localSizes[sc] != nil,
			PullTag:   base + ":" + sc,
		}
		if acc := localSizes[sc]; acc != nil {
			pill.Installed = true
			pill.DiskBytes = acc.bytes
			pill.DiskHuman = ollama.FormatSize(acc.bytes)
			pill.Quant = acc.quant
			pill.LocalTags = acc.tags
			pill.Available = true
		}
		// if only on library
		if !pill.Installed && libSizeSet[sc] {
			pill.Available = true
		}
		f.Sizes = append(f.Sizes, pill)
	}

	return f
}

func sizeRank(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := 1.0
	if strings.HasSuffix(s, "m") {
		mult = 0.001
		s = strings.TrimSuffix(s, "m")
	} else {
		s = strings.TrimSuffix(s, "b")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 1e12 // unknown last
	}
	return f * mult
}

// CatalogEnricher adapts catalog.Client to Enricher.
type CatalogEnricher struct {
	C *catalog.Client
}

func (e CatalogEnricher) FamilyPills(ctx context.Context, base string) (catalog.FamilyPills, error) {
	if e.C == nil {
		return catalog.FamilyPills{}, nil
	}
	return e.C.FamilyPills(ctx, base)
}
