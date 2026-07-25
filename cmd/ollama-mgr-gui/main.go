package main

import (
	"github.com/guysc/ollama-mgr/internal/config"
	"github.com/guysc/ollama-mgr/internal/ui/gui"
)

func main() {
	cfg := config.Default()
	gui.Run(cfg)
}
