package main

import (
	"flag"
	"os"

	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/kilrkrow/ollama-mgr/internal/ui/gui"
)

func main() {
	httpOnly := flag.Bool("http", false, "serve UI over HTTP only (no WebView; for screenshots/dev)")
	addr := flag.String("addr", "127.0.0.1:8765", "listen address when -http is set")
	flag.Parse()

	cfg := config.Default()
	if *httpOnly || os.Getenv("OLLAMA_MGR_HTTP") == "1" {
		gui.RunHTTP(cfg, *addr)
		return
	}
	gui.Run(cfg)
}
