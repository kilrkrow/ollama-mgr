package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/guysc/ollama-mgr/internal/actions"
	"github.com/guysc/ollama-mgr/internal/catalog"
	"github.com/guysc/ollama-mgr/internal/config"
	"github.com/guysc/ollama-mgr/internal/family"
	"github.com/guysc/ollama-mgr/internal/jobs"
	"github.com/guysc/ollama-mgr/internal/modelparse"
	"github.com/guysc/ollama-mgr/internal/ollama"
	"github.com/guysc/ollama-mgr/internal/registry"
	"github.com/guysc/ollama-mgr/internal/upgrade"
	"github.com/jchv/go-webview2"
)

// Run starts a local API server and opens a WebView2 window (falls back to browser).
func Run(cfg config.Config) {
	_ = cfg.EnsureDirs()
	srv := &server{
		cfg:    cfg,
		client: ollama.New(cfg.Endpoint),
		jobs:   jobs.NewManager(),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	addr := ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/list", srv.handleList)
	mux.HandleFunc("/api/families", srv.handleFamilies)
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

type server struct {
	cfg    config.Config
	client *ollama.Client
	jobs   *jobs.Manager
	mu     sync.Mutex
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
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
		Name      string `json:"name"`
		Size      string `json:"size"`
		SizeBytes int64  `json:"size_bytes"`
		Params    string `json:"params"`
		Quant     string `json:"quant"`
		Released  string `json:"released"` // upstream library Updated date
		Modified  string `json:"modified"` // local pull/modified
		Library   string `json:"library"`
		Status    string `json:"status"`
	}
	out := make([]row, 0, len(models))
	var total int64
	for _, m := range models {
		p := modelparse.Parse(m.Name, m.ParameterSize)
		released := "—"
		if meta, ok := upstream[m.Name]; ok && !meta.UpdatedAt.IsZero() {
			released = meta.UpdatedAt.UTC().Format("2006-01-02")
		}
		out = append(out, row{
			Name:      m.Name,
			Size:      ollama.FormatSize(m.Size),
			SizeBytes: m.Size,
			Params:    m.ParameterSize,
			Quant:     m.QuantizationLevel,
			Released:  released,
			Modified:  m.ModifiedAt.Local().Format("2006-01-02"),
			Library:   p.LibraryURL(),
			Status:    "—",
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
	fams := family.Group(ctx, models, family.CatalogEnricher{C: cat})
	var total int64
	for _, f := range fams {
		total += f.DiskBytes
	}
	writeJSON(w, map[string]any{
		"families": fams,
		"count":    len(fams),
		"tags":     len(models),
		"total":    ollama.FormatSize(total),
	})
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
				st = "NOTIONAL → " + res.Candidates[0].FullName
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
		"message": "upgrade started — watch job status (swap deletes only after verify)",
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
	// Launch in new console so GUI stays responsive
	cmd := exec.Command("cmd", "/c", "start", "cmd", "/k", "ollama", "run", body.Name)
	if err := cmd.Start(); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"ok": "started " + body.Name})
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
