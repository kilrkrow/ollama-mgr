package main

import (
	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/kilrkrow/ollama-mgr/internal/ui/gui"
)

func main() {
	cfg := config.Default()
	gui.Run(cfg)
}
