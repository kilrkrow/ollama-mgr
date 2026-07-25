package gui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jchv/go-webview2"
	"github.com/kilrkrow/ollama-mgr/internal/actions"
	"github.com/kilrkrow/ollama-mgr/internal/catalog"
	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/kilrkrow/ollama-mgr/internal/family"
	"github.com/kilrkrow/ollama-mgr/internal/jobs"
	"github.com/kilrkrow/ollama-mgr/internal/modelparse"
	"github.com/kilrkrow/ollama-mgr/internal/ollama"
	"github.com/kilrkrow/ollama-mgr/internal/origin"
	"github.com/kilrkrow/ollama-mgr/internal/registry"
	"github.com/kilrkrow/ollama-mgr/internal/upgrade"
)

//go:embed static/*
var staticFS embed.FS

// Run starts a local API server and opens a WebView2 window (falls back to browser).
func Run(cfg config.Config) {
	addr, stop := startServer(cfg, "127.0.0.1:0")
	defer stop()

	url := "http://" + addr + "/"
	// Prefer native WebView2 window; fall back to default browser.
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "ollama-mgr",
			Width:  1100,
			Height: 640,
			Center: true,
		},
	})
	if w == nil {
		log.Printf("WebView2 unavailable; opening browser at %s", url)
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		// keep process alive serving HTTP
		select {}
	}
	defer w.Destroy()
	w.Navigate(url)
	w.Run()
}

// RunHTTP serves the GUI over HTTP only (no WebView). Used for screenshots/dev.
// addr example: "127.0.0.1:8765". Blocks until stop is impossible — call from main.
func RunHTTP(cfg config.Config, addr string) {
	listen, stop := startServer(cfg, addr)
	defer stop()
	log.Printf("ollama-mgr HTTP UI at http://%s/", listen)
	select {}
}

func startServer(cfg config.Config, listenAddr string) (bound string, stop func()) {
	_ = cfg.EnsureDirs()
	fetched := map[string]bool{}
	for _, b := range cfg.LoadFetchedBases() {
		fetched[b] = true
	}
	srv := &server{
		cfg:          cfg,
		client:       ollama.New(cfg.Endpoint),
		jobs:         jobs.NewManager(),
		fetchedBases: fetched,
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	bound = ln.Addr().String()
	mux := http.NewServeMux()
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/list", srv.handleList)
	mux.HandleFunc("/api/families", srv.handleFamilies)
	mux.HandleFunc("/api/library/search", srv.handleLibrarySearch)
	mux.HandleFunc("/api/families/fetch", srv.handleFamilyFetch)
	mux.HandleFunc("/api/popular", srv.handlePopular)
	mux.HandleFunc("/api/status", srv.handleStatus)
	mux.HandleFunc("/api/check", srv.handleCheck)
	mux.HandleFunc("/api/delete", srv.handleDelete)
	mux.HandleFunc("/api/upgrade", srv.handleUpgrade)
	mux.HandleFunc("/api/jobs", srv.handleJobs)
	mux.HandleFunc("/api/open", srv.handleOpen)
	mux.HandleFunc("/api/run", srv.handleRun)
	mux.HandleFunc("/api/serve", srv.handleServe)

	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Printf("http server: %v", err)
		}
	}()
	return bound, func() { _ = ln.Close() }
}

type server struct {
	cfg    config.Config
	client *ollama.Client
	jobs   *jobs.Manager
	mu     sync.Mutex
	// fetchedBases: library families added via "+" (session board; may have zero local tags).
	fetchedBases map[string]bool
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *server) handlePopular(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	top, _ := strconv.Atoi(r.URL.Query().Get("top"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize <= 0 {
		pageSize = 10
	}
	top = catalog.NormalizeTop(top)

	_ = s.cfg.EnsureDirs()
	cat := catalog.New(s.cfg.CacheDir, s.cfg.CacheTTL)
	all, err := cat.Popular(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	pageItems, total := catalog.PopularPage(all, top, page, pageSize)

	// Local size presence for outline vs solid
	localByBase := map[string]map[string]bool{}
	if models, err := s.client.List(ctx); err == nil {
		for _, m := range models {
			p := modelparse.Parse(m.Name, m.ParameterSize)
			if localByBase[p.BaseName] == nil {
				localByBase[p.BaseName] = map[string]bool{}
			}
			if p.SizeClass != "" {
				localByBase[p.BaseName][p.SizeClass] = true
			}
		}
	}

	type item struct {
		Rank            int               `json:"rank"`
		Name            string            `json:"name"`
		Pulls           string            `json:"pulls,omitempty"`
		URL             string            `json:"url"`
		Features        []string          `json:"features"`
		Sizes           []string          `json:"sizes"`
		InstalledSizes  map[string]bool   `json:"installed_sizes"`
		Origin          origin.Info       `json:"origin"`
	}
	out := make([]item, 0, len(pageItems))
	// Enrich only this page (lazy)
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	sem := make(chan struct{}, 4)
	results := make([]item, len(pageItems))
	for i, pi := range pageItems {
		i, pi := i, pi
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			it := item{
				Rank:           pi.Rank,
				Name:           pi.Name,
				Pulls:          pi.Pulls,
				URL:            pi.URL,
				Origin:         origin.Lookup(pi.Name),
				InstalledSizes: localByBase[pi.Name],
			}
			if it.InstalledSizes == nil {
				it.InstalledSizes = map[string]bool{}
			}
			if pills, err := cat.FamilyPills(ctx, pi.Name); err == nil {
				it.Features = pills.Features
				it.Sizes = pills.Sizes
			}
			mu.Lock()
			results[i] = it
			mu.Unlock()
		}()
	}
	wg.Wait()
	out = append(out, results...)

	writeJSON(w, map[string]any{
		"top":       top,
		"page":      page,
		"page_size": pageSize,
		"total":     total,
		"items":     out,
	})
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	models, err := s.client.List(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Library "Updated" timestamps (release-ish dates), cached + parallel.
	cat := catalog.New(s.cfg.CacheDir, s.cfg.CacheTTL)
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.Name
	}
	upstream := cat.UpstreamUpdatedBatch(ctx, names)

	type row struct {
		Name      string      `json:"name"`
		Size      string      `json:"size"`
		SizeBytes int64       `json:"size_bytes"`
		Params    string      `json:"params"`
		Quant     string      `json:"quant"`
		Released  string      `json:"released"` // upstream library Updated date
		Modified  string      `json:"modified"` // local pull/modified
		Library   string      `json:"library"`
		Status    string      `json:"status"`
		Origin    origin.Info `json:"origin"`
		Flag      string      `json:"flag"`
	}
	out := make([]row, 0, len(models))
	var total int64
	for _, m := range models {
		p := modelparse.Parse(m.Name, m.ParameterSize)
		released := "â€”"
		if meta, ok := upstream[m.Name]; ok && !meta.UpdatedAt.IsZero() {
			released = meta.UpdatedAt.UTC().Format("2006-01-02")
		}
		oi := origin.Lookup(p.BaseName)
		out = append(out, row{
			Name:      m.Name,
			Size:      ollama.FormatSize(m.Size),
			SizeBytes: m.Size,
			Params:    m.ParameterSize,
			Quant:     m.QuantizationLevel,
			Released:  released,
			Modified:  m.ModifiedAt.Local().Format("2006-01-02"),
			Library:   p.LibraryURL(),
			Status:    "-",
			Origin:    oi,
			Flag:      oi.Flag,
		})
		total += m.Size
	}
	writeJSON(w, map[string]any{
		"models": out,
		"total":  ollama.FormatSize(total),
		"count":  len(out),
	})
}

func (s *server) handleFamilies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	models, err := s.client.List(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	_ = s.cfg.EnsureDirs()
	cat := catalog.New(s.cfg.CacheDir, s.cfg.CacheTTL)
	fetched := s.listFetched()
	fams := family.GroupWithFetched(ctx, models, fetched, family.CatalogEnricher{C: cat})
	// Drop fetched bases that are now on disk from the session board (optional cleanup)
	s.pruneFetchedOnDisk(fams)
	var total int64
	for _, f := range fams {
		total += f.DiskBytes
	}
	writeJSON(w, map[string]any{
		"families": fams,
		"count":    len(fams),
		"tags":     len(models),
		"total":    ollama.FormatSize(total),
		"fetched":  s.listFetched(),
	})
}

func (s *server) listFetched() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.fetchedBases))
	for b := range s.fetchedBases {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

func (s *server) persistFetchedLocked() {
	// caller holds s.mu
	bases := make([]string, 0, len(s.fetchedBases))
	for b := range s.fetchedBases {
		bases = append(bases, b)
	}
	_ = s.cfg.SaveFetchedBases(bases)
}

func (s *server) pruneFetchedOnDisk(fams []family.Family) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, f := range fams {
		if f.OnDisk {
			if _, ok := s.fetchedBases[f.Base]; ok {
				delete(s.fetchedBases, f.Base)
				changed = true
			}
		}
	}
	if changed {
		s.persistFetchedLocked()
	}
}

func (s *server) handleLibrarySearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, map[string]any{"query": q, "results": []any{}, "exact": ""})
		return
	}
	_ = s.cfg.EnsureDirs()
	cat := catalog.New(s.cfg.CacheDir, s.cfg.CacheTTL)
	entries, err := cat.Search(r.Context(), q)
	if err != nil {
		writeErr(w, err)
		return
	}
	type hit struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	results := make([]hit, 0, len(entries))
	exact := ""
	ql := strings.ToLower(q)
	// strip accidental tag
	if i := strings.Index(ql, ":"); i >= 0 {
		ql = ql[:i]
	}
	for _, e := range entries {
		name := e.Name
		results = append(results, hit{Name: name, URL: "https://ollama.com/library/" + name})
		if strings.EqualFold(name, ql) {
			exact = name
		}
	}
	// Also accept exact base even if search ranking is weird: probe FamilyPills
	if exact == "" {
		if p, err := cat.FamilyPills(r.Context(), ql); err == nil && (len(p.Sizes) > 0 || p.Name != "") {
			// verify page is real by sizes
			if len(p.Sizes) > 0 {
				exact = ql
				// ensure in results
				found := false
				for _, h := range results {
					if strings.EqualFold(h.Name, exact) {
						found = true
						break
					}
				}
				if !found {
					results = append([]hit{{Name: exact, URL: "https://ollama.com/library/" + exact}}, results...)
				}
			}
		}
	}
	writeJSON(w, map[string]any{
		"query":   q,
		"results": results,
		"exact":   exact,
	})
}

func (s *server) handleFamilyFetch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = s.cfg.EnsureDirs()
	cat := catalog.New(s.cfg.CacheDir, s.cfg.CacheTTL)
	enrich := family.CatalogEnricher{C: cat}

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
			writeErr(w, fmt.Errorf("name required"))
			return
		}
		name := strings.TrimSpace(body.Name)
		if i := strings.Index(name, ":"); i >= 0 {
			name = name[:i]
		}
		// Prefer exact library identity from search
		entries, _ := cat.Search(ctx, name)
		resolved := ""
		for _, e := range entries {
			if strings.EqualFold(e.Name, name) {
				resolved = e.Name
				break
			}
		}
		if resolved == "" {
			// allow direct fetch if FamilyPills has sizes
			f, err := family.FetchLibraryFamily(ctx, name, enrich)
			if err != nil {
				writeErr(w, fmt.Errorf("no exact library match for %q (pick a result from search)", name))
				return
			}
			resolved = f.Base
			s.mu.Lock()
			s.fetchedBases[resolved] = true
			s.persistFetchedLocked()
			s.mu.Unlock()
			writeJSON(w, map[string]any{"family": f, "added": resolved})
			return
		}
		f, err := family.FetchLibraryFamily(ctx, resolved, enrich)
		if err != nil {
			writeErr(w, err)
			return
		}
		// If already on disk, still return merged view
		models, _ := s.client.List(ctx)
		for _, m := range models {
			p := modelparse.Parse(m.Name, m.ParameterSize)
			if strings.EqualFold(p.BaseName, resolved) {
				// merge via GroupWithFetched
				s.mu.Lock()
				s.fetchedBases[resolved] = true
				s.persistFetchedLocked()
				s.mu.Unlock()
				fams := family.GroupWithFetched(ctx, models, s.listFetched(), enrich)
				for _, fam := range fams {
					if strings.EqualFold(fam.Base, resolved) {
						writeJSON(w, map[string]any{"family": fam, "added": resolved, "already_on_disk": true})
						return
					}
				}
			}
		}
		s.mu.Lock()
		s.fetchedBases[resolved] = true
		s.persistFetchedLocked()
		s.mu.Unlock()
		writeJSON(w, map[string]any{"family": f, "added": resolved})

	case http.MethodDelete:
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeErr(w, fmt.Errorf("name required"))
			return
		}
		s.mu.Lock()
		delete(s.fetchedBases, name)
		s.persistFetchedLocked()
		s.mu.Unlock()
		writeJSON(w, map[string]any{"removed": name})

	default:
		http.Error(w, "POST or DELETE", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	err := s.client.Ping(ctx)
	up := err == nil
	msg := "UP"
	if !up {
		msg = "DOWN: " + err.Error()
	}
	writeJSON(w, map[string]any{
		"up":       up,
		"endpoint": s.cfg.Endpoint,
		"message":  msg,
	})
}

func (s *server) handleCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	models, err := s.client.List(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Optional filter: { "names": ["a:tag", "b:tag"] }
	if r.Method == http.MethodPost {
		var body struct {
			Names []string `json:"names"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && len(body.Names) > 0 {
			want := map[string]bool{}
			for _, n := range body.Names {
				want[n] = true
			}
			filtered := models[:0]
			for _, m := range models {
				if want[m.Name] {
					filtered = append(filtered, m)
				}
			}
			models = filtered
		}
	}
	eng := &upgrade.Engine{
		Ollama:   s.client,
		Registry: registry.New(),
		Catalog:  catalog.New(s.cfg.CacheDir, s.cfg.CacheTTL),
	}
	results := eng.CheckAll(ctx, models, upgrade.Options{
		CheckDigest:   true,
		CheckNotional: true,
		Pinned:        s.cfg.IsPinned,
		MaxCandidates: 3,
	})
	// map name -> status string + candidates
	type item struct {
		Model      string              `json:"model"`
		Kind       string              `json:"kind"`
		Status     string              `json:"status"`
		Message    string              `json:"message"`
		Candidates []upgrade.Candidate `json:"candidates,omitempty"`
		LibraryURL string              `json:"library_url"`
	}
	items := make([]item, 0, len(results))
	attention := 0
	for _, res := range results {
		st := string(res.Kind)
		switch res.Kind {
		case upgrade.KindDigest:
			st = "UPDATE (digest)"
			attention++
		case upgrade.KindNotional:
			if len(res.Candidates) > 0 {
				st = "NOTIONAL â†’ " + res.Candidates[0].FullName
			} else {
				st = "NOTIONAL"
			}
			attention++
		case upgrade.KindOK:
			st = "OK"
		default:
			st = strings.ToUpper(string(res.Kind))
		}
		items = append(items, item{
			Model:      res.Model,
			Kind:       string(res.Kind),
			Status:     st,
			Message:    res.Message,
			Candidates: res.Candidates,
			LibraryURL: res.LibraryURL,
		})
	}
	writeJSON(w, map[string]any{"results": items, "attention": attention})
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name  string   `json:"name"`
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, fmt.Errorf("invalid body"))
		return
	}
	names := body.Names
	if body.Name != "" {
		names = append(names, body.Name)
	}
	if len(names) == 0 {
		writeErr(w, fmt.Errorf("name or names required"))
		return
	}
	var deleted []string
	var failed []string
	for _, name := range names {
		if err := s.client.Delete(r.Context(), name); err != nil {
			failed = append(failed, name+": "+err.Error())
			continue
		}
		deleted = append(deleted, name)
	}
	writeJSON(w, map[string]any{
		"deleted": deleted,
		"failed":  failed,
		"ok":      fmt.Sprintf("deleted %d of %d", len(deleted), len(names)),
	})
}

func (s *server) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	req := actions.UpgradeRequest{
		From: body.From,
		To:   body.To,
		Mode: actions.Mode(body.Mode),
	}
	if req.Mode == actions.ModePull && req.To == "" {
		req.To = req.From
	}
	if req.Mode == actions.ModeSideBySide && req.To == "" {
		writeErr(w, fmt.Errorf("side-by-side requires target model"))
		return
	}
	// Async staged job: UI tags pending-delete + download row immediately.
	id := s.jobs.StartUpgrade(s.client, req)
	job, _ := s.jobs.Get(id)
	writeJSON(w, map[string]any{
		"job_id":  id,
		"job":     job,
		"message": "upgrade started â€” watch job status (swap deletes only after verify)",
	})
}

func (s *server) handleJobs(w http.ResponseWriter, r *http.Request) {
	list := s.jobs.List()
	writeJSON(w, map[string]any{"jobs": list})
}

func (s *server) handleOpen(w http.ResponseWriter, r *http.Request) {
	// GET ?name= or POST {names:[]}
	var names []string
	if r.Method == http.MethodPost {
		var body struct {
			Names []string `json:"names"`
			Name  string   `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		names = body.Names
		if body.Name != "" {
			names = append(names, body.Name)
		}
	} else if n := r.URL.Query().Get("name"); n != "" {
		names = []string{n}
	}
	if len(names) == 0 {
		writeErr(w, fmt.Errorf("name required"))
		return
	}
	var urls []string
	for _, name := range names {
		p := modelparse.Parse(name, "")
		u := p.LibraryURL()
		urls = append(urls, u)
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	}
	writeJSON(w, map[string]any{"urls": urls})
}

func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeErr(w, fmt.Errorf("name required"))
		return
	}
	// Launch interactive chat in a *new* console window.
	//
	// Windows START treats the first *quoted* token as the window title.
	// Wrong:  start "ollama run mistral:7b"   â†’ tries to execute that string as a program
	//          (Go re-quotes a single /C string and breaks nested quotes)
	// Right:  start "" cmd.exe /K ollama run mistral:7b
	//          empty title "", then real executable cmd.exe
	cmd := exec.Command(
		"cmd.exe", "/C",
		"start", "",
		"cmd.exe", "/K",
		"ollama", "run", body.Name,
	)
	if err := cmd.Start(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"ok": "opened console for ollama run " + body.Name})
}

func (s *server) handleServe(w http.ResponseWriter, r *http.Request) {
	if err := s.client.Ping(r.Context()); err == nil {
		writeJSON(w, map[string]string{"message": "already up"})
		return
	}
	if err := ollama.StartServe(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"message": "serve started"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
