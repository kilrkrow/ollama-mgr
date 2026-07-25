package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kilrkrow/ollama-mgr/internal/modelparse"
)

// Entry is a model discovered on ollama.com.
type Entry struct {
	Name     string   `json:"name"`
	Sizes    []string `json:"sizes"`
	Pulls    string   `json:"pulls,omitempty"`
	Updated  string   `json:"updated,omitempty"`
	FullURL  string   `json:"url,omitempty"`
}

// Client discovers library models via ollama.com HTML (cached).
type Client struct {
	HTTPClient *http.Client
	CacheDir   string
	CacheTTL   time.Duration
}

// New creates a catalog client.
func New(cacheDir string, ttl time.Duration) *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
		CacheDir:   cacheDir,
		CacheTTL:   ttl,
	}
}

var (
	// href="/library/qwen3-coder" or title="qwen3.5"
	libHrefRe   = regexp.MustCompile(`href="/library/([a-zA-Z0-9._-]+)"`)
	sizeBadgeRe = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?[bm])\b`)
	tagHrefRe   = regexp.MustCompile(`href="/library/([a-zA-Z0-9._/-]+):([a-zA-Z0-9._-]+)"`)
	// <span ... title="May 28, 2025 1:19 AM UTC"> ... Updated
	updatedTitleRe = regexp.MustCompile(`title="([A-Za-z]{3} \d{1,2}, \d{4} \d{1,2}:\d{2} [AP]M UTC)"`)
)

// UpstreamMeta is the library "Updated" timestamp for a model tag (closest thing
// to a release date Ollama publishes publicly).
type UpstreamMeta struct {
	Model     string    `json:"model"`
	UpdatedAt time.Time `json:"updated_at"`
	Raw       string    `json:"raw,omitempty"`
	URL       string    `json:"url,omitempty"`
}

// UpstreamUpdated fetches the library page Updated timestamp for name (e.g. qwen2.5-coder:32b).
// Cached under CacheDir. Returns zero time if unavailable.
func (c *Client) UpstreamUpdated(ctx context.Context, fullName string) (UpstreamMeta, error) {
	p := modelparse.Parse(fullName, "")
	libPath := p.BaseName
	if p.Namespace != "" && p.Namespace != "library" {
		libPath = p.Namespace + "/" + p.BaseName
	}
	pageKey := libPath
	if p.Tag != "" && p.Tag != "latest" {
		pageKey = libPath + ":" + p.Tag
	} else if p.Tag == "latest" {
		// still try with :latest first; fallback to base page
		pageKey = libPath
	}

	cacheKey := "updated_" + sanitizeKey(pageKey) + ".json"
	if meta, ok := c.loadMetaCache(cacheKey); ok {
		return meta, nil
	}

	// Prefer exact tag page (has per-tag Updated title)
	candidates := []string{}
	if p.Tag != "" {
		candidates = append(candidates, libPath+":"+p.Tag)
	}
	candidates = append(candidates, libPath)

	var lastErr error
	for _, path := range candidates {
		u := "https://ollama.com/library/" + path
		body, err := c.fetch(ctx, u)
		if err != nil {
			lastErr = err
			continue
		}
		raw, t, ok := parseUpdatedTitle(body)
		if !ok {
			lastErr = fmt.Errorf("no updated date on %s", path)
			continue
		}
		meta := UpstreamMeta{
			Model:     fullName,
			UpdatedAt: t,
			Raw:       raw,
			URL:       u,
		}
		_ = c.saveMetaCache(cacheKey, meta)
		return meta, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("upstream date unavailable")
	}
	return UpstreamMeta{Model: fullName}, lastErr
}

// UpstreamUpdatedBatch looks up dates for many models in parallel (bounded).
func (c *Client) UpstreamUpdatedBatch(ctx context.Context, names []string) map[string]UpstreamMeta {
	out := make(map[string]UpstreamMeta, len(names))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			meta, err := c.UpstreamUpdated(ctx, name)
			if err != nil {
				return
			}
			mu.Lock()
			out[name] = meta
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func parseUpdatedTitle(html string) (raw string, t time.Time, ok bool) {
	m := updatedTitleRe.FindStringSubmatch(html)
	if m == nil {
		return "", time.Time{}, false
	}
	raw = m[1]
	// Example: May 28, 2025 1:19 AM UTC
	layouts := []string{
		"Jan 2, 2006 3:04 PM MST",
		"Jan 02, 2006 3:04 PM MST",
		"January 2, 2006 3:04 PM MST",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return raw, parsed.UTC(), true
		}
	}
	return raw, time.Time{}, false
}

type metaCacheFile struct {
	SavedAt time.Time    `json:"saved_at"`
	Meta    UpstreamMeta `json:"meta"`
}

func (c *Client) loadMetaCache(key string) (UpstreamMeta, bool) {
	if c.CacheDir == "" {
		return UpstreamMeta{}, false
	}
	b, err := os.ReadFile(filepath.Join(c.CacheDir, key))
	if err != nil {
		return UpstreamMeta{}, false
	}
	var cf metaCacheFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return UpstreamMeta{}, false
	}
	if time.Since(cf.SavedAt) > c.CacheTTL {
		return UpstreamMeta{}, false
	}
	if cf.Meta.UpdatedAt.IsZero() {
		return UpstreamMeta{}, false
	}
	return cf.Meta, true
}

func (c *Client) saveMetaCache(key string, meta UpstreamMeta) error {
	if c.CacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	cf := metaCacheFile{SavedAt: time.Now(), Meta: meta}
	b, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.CacheDir, key), b, 0o644)
}

// Search queries ollama.com/search and returns entries (from cache when fresh).
func (c *Client) Search(ctx context.Context, query string) ([]Entry, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	cacheKey := "search_" + sanitizeKey(query) + ".json"
	if entries, ok := c.loadCache(cacheKey); ok {
		return entries, nil
	}

	u := "https://ollama.com/search?q=" + url.QueryEscape(query)
	body, err := c.fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	entries := parseSearchHTML(body)
	_ = c.saveCache(cacheKey, entries)
	return entries, nil
}

// FamilyPills is homepage-style feature + size chips for a library model.
type FamilyPills struct {
	Name     string   `json:"name"`
	Features []string `json:"features"` // tools, vision, thinking, ...
	Sizes    []string `json:"sizes"`    // 30b, 7b, ...
	URL      string   `json:"url"`
}

// feature/size pill classes on ollama.com library pages
var (
	featurePillRe = regexp.MustCompile(`(?i)bg-indigo-50[^>]*>([a-z0-9._-]+)<`)
	sizePillRe    = regexp.MustCompile(`(?i)bg-\[#ddf4ff\][^>]*>([a-z0-9._-]+)<`)
	// fallback: any short token pill near top
	genericSizeTokenRe = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?[bm])$`)
)

// FamilyPills fetches feature and canonical size pills from /library/{name}.
func (c *Client) FamilyPills(ctx context.Context, name string) (FamilyPills, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return FamilyPills{}, nil
	}
	cacheKey := "familypills_" + sanitizeKey(name) + ".json"
	if p, ok := c.loadFamilyPillsCache(cacheKey); ok {
		return p, nil
	}

	u := "https://ollama.com/library/" + name
	body, err := c.fetch(ctx, u)
	if err != nil {
		// fallback: derive sizes from tags page only
		tags, terr := c.TagsForModel(ctx, name)
		if terr != nil {
			return FamilyPills{}, err
		}
		p := FamilyPills{
			Name:  name,
			Sizes: CanonicalSizePills(tags),
			URL:   u,
		}
		_ = c.saveFamilyPillsCache(cacheKey, p)
		return p, nil
	}

	features := uniqueLower(featurePillRe.FindAllStringSubmatch(body, -1))
	sizes := uniqueLower(sizePillRe.FindAllStringSubmatch(body, -1))
	// filter features vs sizes if misclassified
	var featOut, sizeOut []string
	for _, f := range features {
		if genericSizeTokenRe.MatchString(f) {
			sizeOut = append(sizeOut, f)
			continue
		}
		// known feature-ish tokens
		featOut = append(featOut, f)
	}
	for _, s := range sizes {
		if genericSizeTokenRe.MatchString(s) {
			sizeOut = append(sizeOut, s)
		}
	}
	if len(sizeOut) == 0 {
		// tags page fallback for sizes
		if tags, err := c.TagsForModel(ctx, name); err == nil {
			sizeOut = CanonicalSizePills(tags)
		}
	} else {
		sizeOut = CanonicalSizePills(sizeOut)
	}

	p := FamilyPills{
		Name:     name,
		Features: featOut,
		Sizes:    sizeOut,
		URL:      u,
	}
	_ = c.saveFamilyPillsCache(cacheKey, p)
	return p, nil
}

// CanonicalSizePills keeps pure size tokens (7b, 30b, 480b), drops quant soup / cloud.
func CanonicalSizePills(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || t == "latest" {
			continue
		}
		if strings.Contains(t, "cloud") {
			continue
		}
		// pure size tag
		if genericSizeTokenRe.MatchString(t) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
			continue
		}
		// extract leading size from composite only if whole tag is size-like prefix
		// e.g. 30b-a3b â†’ treat as 30b for matrix? skip composites for homepage-style pills
	}
	return out
}

func uniqueLower(matches [][]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		s := strings.ToLower(strings.TrimSpace(m[1]))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

type familyPillsCache struct {
	SavedAt time.Time   `json:"saved_at"`
	Pills   FamilyPills `json:"pills"`
}

func (c *Client) loadFamilyPillsCache(key string) (FamilyPills, bool) {
	if c.CacheDir == "" {
		return FamilyPills{}, false
	}
	b, err := os.ReadFile(filepath.Join(c.CacheDir, key))
	if err != nil {
		return FamilyPills{}, false
	}
	var cf familyPillsCache
	if err := json.Unmarshal(b, &cf); err != nil {
		return FamilyPills{}, false
	}
	if time.Since(cf.SavedAt) > c.CacheTTL {
		return FamilyPills{}, false
	}
	return cf.Pills, true
}

func (c *Client) saveFamilyPillsCache(key string, p FamilyPills) error {
	if c.CacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	cf := familyPillsCache{SavedAt: time.Now(), Pills: p}
	b, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.CacheDir, key), b, 0o644)
}

// TagsForModel fetches size/role tags for a library model name.
func (c *Client) TagsForModel(ctx context.Context, name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	cacheKey := "tags_" + sanitizeKey(name) + ".json"
	if entries, ok := c.loadCache(cacheKey); ok && len(entries) == 1 {
		return entries[0].Sizes, nil
	}

	u := "https://ollama.com/library/" + name + "/tags"
	body, err := c.fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	tags := parseTagsHTML(body, name)
	_ = c.saveCache(cacheKey, []Entry{{Name: name, Sizes: tags}})
	return tags, nil
}

// FindSuccessors searches for newer same-family models matching specialty/size.
func (c *Client) FindSuccessors(ctx context.Context, installed modelparse.Parsed, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 5
	}
	q := installed.Family
	if installed.Specialty != "" {
		q = installed.Family + " " + installed.Specialty
	}
	if q == "" {
		q = installed.BaseName
	}
	entries, err := c.Search(ctx, q)
	if err != nil {
		return nil, err
	}

	var out []Entry
	for _, e := range entries {
		if strings.EqualFold(e.Name, installed.BaseName) {
			continue
		}
		cand := modelparse.Parse(e.Name+":latest", "")
		if cand.Family != installed.Family {
			continue
		}
		if !specialtyCompatible(installed.Specialty, cand.Specialty) {
			continue
		}
		if cand.Version.IsZero() || installed.Version.IsZero() {
			// still allow if name clearly different and family matches â€” require newer version when both parse
			if !installed.Version.IsZero() && cand.Version.IsZero() {
				continue
			}
			if !cand.Version.IsZero() && installed.Version.IsZero() {
				// candidate has version, installed doesn't â€” skip notional
				continue
			}
		} else if cand.Version.Compare(installed.Version) <= 0 {
			continue
		}
		// ensure size tags available (search HTML often omits badges)
		sizes := e.Sizes
		if tags, err := c.TagsForModel(ctx, e.Name); err == nil && len(tags) > 0 {
			tagSizes := filterSizeTags(tags)
			// merge
			seen := map[string]bool{}
			var merged []string
			for _, s := range append(sizes, tagSizes...) {
				if s == "" || seen[s] {
					continue
				}
				seen[s] = true
				merged = append(merged, s)
			}
			sizes = merged
			e.Sizes = sizes
		}
		if installed.SizeClass != "" {
			if !hasCompatibleSize(sizes, installed.SizeClass) {
				continue
			}
		}
		e.FullURL = "https://ollama.com/library/" + e.Name
		out = append(out, e)
		if len(out) >= limit*3 {
			break
		}
	}

	// rank by version desc
	sortEntriesByVersion(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func specialtyCompatible(a, b string) bool {
	if a == "" && b == "" {
		return true
	}
	// if installed has specialty, candidate must match
	if a != "" {
		return a == b
	}
	// installed is general: allow general candidates only (avoid suggesting coder for base)
	return b == ""
}

func hasCompatibleSize(sizes []string, want string) bool {
	for _, s := range sizes {
		ns := normalizeSizeToken(s)
		if modelparse.SizeCompatible(ns, want) {
			return true
		}
	}
	return false
}

func filterSizeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		// pure size tags like 32b, 7b
		if m := sizeBadgeRe.FindString(strings.ToLower(t)); m != "" && !strings.Contains(t, "-") {
			n := normalizeSizeToken(m)
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
			continue
		}
		// extract size from composite tags
		if m := sizeBadgeRe.FindString(strings.ToLower(t)); m != "" {
			n := normalizeSizeToken(m)
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

func normalizeSizeToken(s string) string {
	p := modelparse.Parse("x:"+s, s)
	if p.SizeClass != "" {
		return p.SizeClass
	}
	return strings.ToLower(s)
}

func sortEntriesByVersion(entries []Entry) {
	// simple insertion by version
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 {
			vi := modelparse.Parse(entries[j].Name+":latest", "").Version
			vj := modelparse.Parse(entries[j-1].Name+":latest", "").Version
			if vi.Compare(vj) > 0 {
				entries[j], entries[j-1] = entries[j-1], entries[j]
				j--
				continue
			}
			break
		}
	}
}

func parseSearchHTML(html string) []Entry {
	// collect unique library names in order
	matches := libHrefRe.FindAllStringSubmatch(html, -1)
	seen := map[string]bool{}
	var names []string
	for _, m := range matches {
		name := m[1]
		// skip tag links that snuck in (contain colon already stripped)
		if strings.Contains(name, ":") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}

	var entries []Entry
	for _, name := range names {
		// find a window around the first occurrence for size badges
		idx := strings.Index(html, `/library/`+name)
		sizes := []string{}
		if idx >= 0 {
			start := idx
			end := idx + 800
			if end > len(html) {
				end = len(html)
			}
			window := html[start:end]
			// size tokens often appear as "32b" badges near the card
			for _, sm := range sizeBadgeRe.FindAllString(strings.ToLower(window), -1) {
				n := normalizeSizeToken(sm)
				// filter noise: years etc â€” require unit b or m and reasonable range
				if len(n) >= 2 && len(n) <= 6 {
					dup := false
					for _, s := range sizes {
						if s == n {
							dup = true
							break
						}
					}
					if !dup {
						sizes = append(sizes, n)
					}
				}
			}
		}
		entries = append(entries, Entry{
			Name:    name,
			Sizes:   sizes,
			FullURL: "https://ollama.com/library/" + name,
		})
	}
	return entries
}

func parseTagsHTML(html, modelName string) []string {
	matches := tagHrefRe.FindAllStringSubmatch(html, -1)
	seen := map[string]bool{}
	var tags []string
	for _, m := range matches {
		if m[1] != modelName && !strings.HasSuffix(m[1], "/"+modelName) {
			// allow exact
			if m[1] != modelName {
				continue
			}
		}
		tag := m[2]
		if seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

func (c *Client) fetch(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ollama-mgr/0.1 (+https://github.com/kilrkrow/ollama-mgr)")
	req.Header.Set("Accept", "text/html")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("catalog fetch %s: %s", rawURL, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type cacheFile struct {
	SavedAt time.Time `json:"saved_at"`
	Entries []Entry   `json:"entries"`
}

func (c *Client) loadCache(key string) ([]Entry, bool) {
	if c.CacheDir == "" {
		return nil, false
	}
	path := filepath.Join(c.CacheDir, key)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cf cacheFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return nil, false
	}
	if time.Since(cf.SavedAt) > c.CacheTTL {
		return nil, false
	}
	return cf.Entries, true
}

func (c *Client) saveCache(key string, entries []Entry) error {
	if c.CacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	cf := cacheFile{SavedAt: time.Now(), Entries: entries}
	b, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.CacheDir, key), b, 0o644)
}

func sanitizeKey(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// BestTagForSize picks the best pull tag for a successor at a given size class.
func BestTagForSize(sizes []string, sizeClass string) string {
	if sizeClass == "" {
		return "latest"
	}
	// prefer exact size-only tag
	for _, s := range sizes {
		if modelparse.SizeCompatible(normalizeSizeToken(s), sizeClass) && !strings.Contains(s, "-") {
			return s
		}
	}
	for _, s := range sizes {
		if modelparse.SizeCompatible(normalizeSizeToken(s), sizeClass) {
			return s
		}
	}
	return sizeClass
}
