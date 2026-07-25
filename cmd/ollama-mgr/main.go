package main

import (
	"os"

	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/spf13/cobra"
)

var (
	flagEndpoint string
	cfg          config.Config
)

func main() {
	cfg = config.Default()

	root := &cobra.Command{
		Use:   "ollama-mgr",
		Short: "Manage local Ollama models: list, check updates, delete, pull, run",
		Long: `ollama-mgr is a thin Windows-friendly manager for Ollama models.

It lists installed models, checks for same-tag weight updates and notional
generation upgrades (e.g. qwen2.5-coder:32b -> qwen3-coder:32b), and can
delete, pull, open library pages, or run models.`,
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&flagEndpoint, "endpoint", cfg.Endpoint, "Ollama API endpoint")

	root.AddCommand(
		cmdList(),
		cmdCheck(),
		cmdUpgrade(),
		cmdRm(),
		cmdPull(),
		cmdOpen(),
		cmdRun(),
		cmdStatus(),
		cmdServe(),
		cmdVersion(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
