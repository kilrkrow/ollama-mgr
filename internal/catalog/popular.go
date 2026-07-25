package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// RankedModel is one entry from ollama.com/library popularity order.
type RankedModel struct {
	Rank  int    `json:"rank"`
	Name  string `json:"name"`
	Pulls string `json:"pulls,omitempty"`
	URL   string `json:"url"`
}

var (
	// Card: href="/library/llama3.1" class="group w-full
	libCardRe = regexp.MustCompile(`href="/library/([a-zA-Z0-9._-]+)"\s+class="group w-full`)
	// Pull counts as they appear on the library page (order loosely tracks cards)
	pullsRe = regexp.MustCompile(`([\d.]+[KMB]?)\s*</span>\s*<span[^>]*>\s*&nbsp;Pulls`)
)

// Popular returns models in library page order (download popularity proxy), cached.
func (c *Client) Popular(ctx context.Context) ([]RankedModel, error) {
	cacheKey := "popular_library.json"
	if list, ok := c.loadPopularCache(cacheKey); ok {
		return list, nil
	}
	body, err := c.fetch(ctx, "https://ollama.com/library")
	if err != nil {
		return nil, err
	}
	list := parsePopularHTML(body)
	if len(list) == 0 {
		return nil, fmt.Errorf("no models parsed from library page")
	}
	_ = c.savePopularCache(cacheKey, list)
	return list, nil
}

// PopularPage slices the ranked list and returns page metadata.
func PopularPage(all []RankedModel, top, page, pageSize int) (items []RankedModel, total int) {
	if top <= 0 {
		top = 10
	}
	if top > len(all) {
		top = len(all)
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if page < 0 {
		page = 0
	}
	slice := all[:top]
	total = len(slice)
	start := page * pageSize
	if start >= total {
		return []RankedModel{}, total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return slice[start:end], total
}

func parsePopularHTML(html string) []RankedModel {
	seen := map[string]bool{}
	var out []RankedModel
	for _, m := range libCardRe.FindAllStringSubmatch(html, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, RankedModel{
			Rank: len(out) + 1,
			Name: name,
			URL:  "https://ollama.com/library/" + name,
		})
	}
	// Best-effort: assign pull counts in document order
	pulls := pullsRe.FindAllStringSubmatch(html, -1)
	for i := 0; i < len(out) && i < len(pulls); i++ {
		if len(pulls[i]) >= 2 {
			out[i].Pulls = pulls[i][1]
		}
	}
	return out
}

type popularCacheFile struct {
	SavedAt time.Time     `json:"saved_at"`
	Items   []RankedModel `json:"items"`
}

func (c *Client) loadPopularCache(key string) ([]RankedModel, bool) {
	if c.CacheDir == "" {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(c.CacheDir, key))
	if err != nil {
		return nil, false
	}
	var cf popularCacheFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return nil, false
	}
	if time.Since(cf.SavedAt) > c.CacheTTL {
		return nil, false
	}
	return cf.Items, true
}

func (c *Client) savePopularCache(key string, items []RankedModel) error {
	if c.CacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	cf := popularCacheFile{SavedAt: time.Now(), Items: items}
	b, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.CacheDir, key), b, 0o644)
}

// NormalizeTop clamps top to allowed set.
func NormalizeTop(top int) int {
	switch top {
	case 10, 25, 50, 100:
		return top
	default:
		if top < 10 {
			return 10
		}
		if top > 100 {
			return 100
		}
		// nearest allowed
		for _, n := range []int{10, 25, 50, 100} {
			if top <= n {
				return n
			}
		}
		return 100
	}
}

// StripBOM helps tests/fixtures.
func StripBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}
