package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/kilrkrow/ollama-mgr/internal/modelparse"
	"github.com/kilrkrow/ollama-mgr/internal/ollama"
	"github.com/spf13/cobra"
)

func cmdRm() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:     "rm <model>...",
		Aliases: []string{"delete", "remove"},
		Short:   "Delete local model(s)",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Fprintf(os.Stderr, "About to delete: %s\nRe-run with --yes to confirm.\n", strings.Join(args, ", "))
				return fmt.Errorf("confirmation required")
			}
			ctx := context.Background()
			cl := client()
			for _, name := range args {
				fmt.Printf("Deleting %s...\n", name)
				if err := cl.Delete(ctx, name); err != nil {
					return err
				}
				fmt.Printf("Deleted %s\n", name)
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "confirm deletion")
	return c
}

func cmdPull() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <model>",
		Short: "Pull or update a model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			return client().Pull(ctx, args[0], printPullProgress)
		},
	}
}

func cmdOpen() *cobra.Command {
	return &cobra.Command{
		Use:   "open <model>",
		Short: "Open the model's Ollama library page in a browser",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// try to enrich size from local list
			param := ""
			if models, err := client().List(context.Background()); err == nil {
				for _, m := range models {
					if m.Name == args[0] || strings.HasPrefix(m.Name, args[0]) {
						param = m.ParameterSize
						break
					}
				}
			}
			p := modelparse.Parse(args[0], param)
			return openBrowser(p.LibraryURL())
		},
	}
}

func cmdRun() *cobra.Command {
	return &cobra.Command{
		Use:   "run <model>",
		Short: "Run a model interactively (ollama run)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ollama.RunInteractive(args[0])
		},
	}
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Ollama daemon status and running models",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cl := client()
			ep := flagEndpoint
			if ep == "" {
				ep = cfg.Endpoint
			}
			if err := cl.Ping(ctx); err != nil {
				fmt.Printf("Daemon: DOWN (%s)\nEndpoint: %s\nError: %v\n", ep, ep, err)
				return nil
			}
			fmt.Printf("Daemon: UP\nEndpoint: %s\n", ep)
			models, err := cl.List(ctx)
			if err != nil {
				return err
			}
			var total int64
			for _, m := range models {
				total += m.Size
			}
			fmt.Printf("Installed: %d models (%s)\n", len(models), ollama.FormatSize(total))
			running, err := cl.Ps(ctx)
			if err != nil {
				fmt.Printf("Running: (unavailable: %v)\n", err)
				return nil
			}
			if len(running) == 0 {
				fmt.Println("Running: none")
				return nil
			}
			fmt.Println("Running:")
			for _, r := range running {
				fmt.Printf("  - %s (%s VRAM)\n", r.Name, ollama.FormatSize(r.SizeVRAM))
			}
			return nil
		},
	}
}

func cmdServe() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start ollama serve if the daemon is not running",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cl := client()
			if err := cl.Ping(ctx); err == nil {
				fmt.Println("Ollama is already running.")
				return nil
			}
			fmt.Println("Starting ollama serve in background (no console)...")
			if err := ollama.StartServe(); err != nil {
				return err
			}
			// wait up to ~10s
			for i := 0; i < 20; i++ {
				time.Sleep(500 * time.Millisecond)
				if err := cl.Ping(context.Background()); err == nil {
					fmt.Println("Ollama is up.")
					return nil
				}
			}
			return fmt.Errorf("started process but API not reachable; check Ollama install / port 11434")
		},
	}
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("ollama-mgr %s\n", config.Version)
		},
	}
}

func printPullProgress(p ollama.PullProgress) {
	if p.Total > 0 {
		pct := float64(p.Completed) / float64(p.Total) * 100
		fmt.Fprintf(os.Stderr, "\r%s %.1f%% (%s/%s)    ", p.Status, pct, ollama.FormatSize(p.Completed), ollama.FormatSize(p.Total))
		if p.Completed >= p.Total && p.Status != "" {
			fmt.Fprintln(os.Stderr)
		}
		return
	}
	if p.Status != "" {
		fmt.Fprintf(os.Stderr, "\r%s                    ", p.Status)
	}
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	fmt.Println(rawURL)
	return cmd.Start()
}
