package upgrade

import (
	"context"
	"fmt"
	"sync"

	"github.com/guysc/ollama-mgr/internal/catalog"
	"github.com/guysc/ollama-mgr/internal/modelparse"
	"github.com/guysc/ollama-mgr/internal/ollama"
	"github.com/guysc/ollama-mgr/internal/registry"
)

// Kind classifies an update finding.
type Kind string

const (
	KindOK       Kind = "ok"
	KindDigest   Kind = "digest"   // same tag, newer weights
	KindNotional Kind = "notional" // newer generation same weight class
	KindPinned   Kind = "pinned"
	KindSkip     Kind = "skip"
	KindError    Kind = "error"
)

// Candidate is a suggested successor model.
type Candidate struct {
	Name     string `json:"name"` // base name
	Tag      string `json:"tag"`
	FullName string `json:"full_name"`
	URL      string `json:"url"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
}

// Result is the check outcome for one installed model.
type Result struct {
	Model       string      `json:"model"`
	Kind        Kind        `json:"kind"`
	Message     string      `json:"message"`
	LocalDigest string      `json:"local_digest,omitempty"`
	RemoteDigest string     `json:"remote_digest,omitempty"`
	Candidates  []Candidate `json:"candidates,omitempty"`
	LibraryURL  string      `json:"library_url"`
	Size        int64       `json:"size"`
	ParamSize   string      `json:"parameter_size,omitempty"`
}

// Options controls what the engine checks.
type Options struct {
	CheckDigest   bool
	CheckNotional bool
	Pinned        func(name string) bool
	MaxCandidates int
}

// Engine coordinates digest + notional checks.
type Engine struct {
	Ollama   *ollama.Client
	Registry *registry.Client
	Catalog  *catalog.Client
}

// CheckAll evaluates every local model.
func (e *Engine) CheckAll(ctx context.Context, models []ollama.Model, opt Options) []Result {
	if opt.MaxCandidates <= 0 {
		opt.MaxCandidates = 3
	}
	results := make([]Result, len(models))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i, m := range models {
		wg.Add(1)
		go func(i int, m ollama.Model) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = e.CheckOne(ctx, m, opt)
		}(i, m)
	}
	wg.Wait()
	return results
}

// CheckOne evaluates a single model.
func (e *Engine) CheckOne(ctx context.Context, m ollama.Model, opt Options) Result {
	parsed := modelparse.Parse(m.Name, m.ParameterSize)
	r := Result{
		Model:      m.Name,
		Kind:       KindOK,
		Message:    "up to date",
		LibraryURL: parsed.LibraryURL(),
		Size:       m.Size,
		ParamSize:  m.ParameterSize,
	}

	if opt.Pinned != nil && opt.Pinned(m.Name) {
		r.Kind = KindPinned
		r.Message = "pinned"
		return r
	}

	// Digest check
	if opt.CheckDigest && e.Registry != nil {
		show, err := e.Ollama.Show(ctx, m.Name)
		local := ""
		if err == nil {
			local = ollama.LocalWeightDigest(show)
		}
		// fallback to list digest
		if local == "" && m.Digest != "" {
			local = normalizeLocalDigest(m.Digest)
		}
		r.LocalDigest = local

		remote, err := e.Registry.ModelDigest(ctx, parsed.RegistryPath(), parsed.Tag)
		if err != nil {
			if r.Kind == KindOK {
				r.Kind = KindSkip
				r.Message = "not in registry or unreachable"
			}
		} else {
			r.RemoteDigest = remote
			if local != "" && !registry.DigestsEqual(local, remote) {
				r.Kind = KindDigest
				r.Message = "weights updated upstream (same tag)"
			} else if local == "" {
				// can't compare
				if r.Kind == KindOK {
					r.Message = "ok (local digest unavailable)"
				}
			}
		}
	}

	// Notional upgrade
	if opt.CheckNotional && e.Catalog != nil && parsed.Family != "" {
		entries, err := e.Catalog.FindSuccessors(ctx, parsed, opt.MaxCandidates)
		if err == nil && len(entries) > 0 {
			var cands []Candidate
			for _, ent := range entries {
				tag := catalog.BestTagForSize(ent.Sizes, parsed.SizeClass)
				// verify size tag exists remotely when possible
				full := ent.Name + ":" + tag
				candParsed := modelparse.Parse(full, "")
				score := scoreCandidate(parsed, candParsed)
				cands = append(cands, Candidate{
					Name:     ent.Name,
					Tag:      tag,
					FullName: full,
					URL:      ent.FullURL,
					Score:    score,
					Reason:   fmt.Sprintf("newer %s series, size %s", candParsed.Version.String(), tag),
				})
			}
			if len(cands) > 0 {
				r.Candidates = cands
				if r.Kind == KindOK || r.Kind == KindSkip {
					r.Kind = KindNotional
					r.Message = "notional upgrade: " + cands[0].FullName
				} else if r.Kind == KindDigest {
					r.Message = r.Message + "; also notional: " + cands[0].FullName
				}
			}
		}
	}

	return r
}

func scoreCandidate(installed, cand modelparse.Parsed) int {
	score := 0
	if !installed.Version.IsZero() && !cand.Version.IsZero() {
		// rough delta on first component
		iv, cv := 0, 0
		if len(installed.Version.Parts) > 0 {
			iv = installed.Version.Parts[0]
		}
		if len(cand.Version.Parts) > 0 {
			cv = cand.Version.Parts[0]
		}
		score += (cv - iv) * 100
		// minor bump
		if len(cand.Version.Parts) > 1 && len(installed.Version.Parts) > 1 {
			score += (cand.Version.Parts[1] - installed.Version.Parts[1]) * 10
		}
	}
	if modelparse.SizeCompatible(installed.SizeClass, cand.SizeClass) || cand.SizeClass == "" {
		score += 10
	}
	if installed.Specialty != "" && installed.Specialty == cand.Specialty {
		score += 10
	}
	return score
}

func normalizeLocalDigest(d string) string {
	d = trimSpace(d)
	if len(d) == 64 {
		return "sha256:" + d
	}
	if len(d) > 7 && d[:7] == "sha256:" {
		return d
	}
	return d
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
