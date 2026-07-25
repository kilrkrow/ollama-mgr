package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kilrkrow/ollama-mgr/internal/actions"
	"github.com/kilrkrow/ollama-mgr/internal/catalog"
	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/kilrkrow/ollama-mgr/internal/family"
	"github.com/kilrkrow/ollama-mgr/internal/modelparse"
	"github.com/kilrkrow/ollama-mgr/internal/ollama"
	"github.com/kilrkrow/ollama-mgr/internal/origin"
	"github.com/kilrkrow/ollama-mgr/internal/registry"
	"github.com/kilrkrow/ollama-mgr/internal/upgrade"
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
generation upgrades (e.g. qwen2.5-coder:32b â†’ qwen3-coder:32b), and can
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

func client() *ollama.Client {
	ep := flagEndpoint
	if ep == "" {
		ep = cfg.Endpoint
	}
	return ollama.New(ep)
}

func parseParamBillions(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	s = strings.ToLower(s)
	mult := 1.0
	if strings.HasSuffix(s, "b") {
		s = strings.TrimSuffix(s, "b")
	} else if strings.HasSuffix(s, "m") {
		s = strings.TrimSuffix(s, "m")
		mult = 0.001
	}
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return f * mult
}

func cmdList() *cobra.Command {
	var (
		asJSON      bool
		withRelease bool
		noRelease   bool
		sortBy      string
		sortDesc    bool
		byFamily    bool
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List installed models",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cl := client()
			models, err := cl.List(ctx)
			if err != nil {
				return err
			}

			if byFamily {
				_ = cfg.EnsureDirs()
				cat := catalog.New(cfg.CacheDir, cfg.CacheTTL)
				fams := family.Group(ctx, models, family.CatalogEnricher{C: cat})
				if asJSON {
					return json.NewEncoder(os.Stdout).Encode(fams)
				}
				var total int64
				for _, f := range fams {
					total += f.DiskBytes
					org := f.Origin
					ctry := org.Code
					if org.Unknown || ctry == "" {
						ctry = "?"
					}
					// Origin only once as [CTRY] prefix — not repeated after the name
					fmt.Printf("[%s] %s\n", ctry, f.Base)
					if org.Org != "" && !org.Unknown {
						fmt.Printf("  origin:   %s · %s\n", org.Name, org.Org)
					}
					// features
					var feats []string
					for _, fp := range f.Features {
						if fp.Name == "completion" && len(f.Features) > 1 {
							continue
						}
						mark := fp.Name
						if fp.Local {
							mark = "[" + mark + "]"
						}
						feats = append(feats, mark)
					}
					if len(feats) > 0 {
						fmt.Printf("  features: %s\n", strings.Join(feats, " "))
					}
					// sizes
					var sizes []string
					for _, sp := range f.Sizes {
						if sp.Installed {
							extra := ""
							if sp.DiskHuman != "" {
								extra = " " + sp.DiskHuman
							}
							if sp.Quant != "" {
								extra += " " + sp.Quant
							}
							sizes = append(sizes, fmt.Sprintf("[%s*]%s", sp.Size, extra))
						} else {
							sizes = append(sizes, sp.Size)
						}
					}
					if len(sizes) > 0 {
						fmt.Printf("  sizes:    %s\n", strings.Join(sizes, "  "))
						fmt.Printf("           [* downloaded]  plain = available on library, not local\n")
					}
					fmt.Printf("  disk:     %s  (%d tag(s))\n", f.DiskHuman, f.TagCount)
					for _, t := range f.Installed {
						fmt.Printf("    - %s\n", t.Name)
					}
					fmt.Println()
				}
				fmt.Printf("%d families, %d tags, %s total\n", len(fams), len(models), ollama.FormatSize(total))
				return nil
			}

			// Default: include upstream library "Updated" dates (cached).
			fetchRelease := true
			if noRelease {
				fetchRelease = false
			}
			if cmd.Flags().Changed("release") {
				fetchRelease = withRelease
			}

			type row struct {
				ollama.Model
				Released   string `json:"released,omitempty"`
				LibraryURL string `json:"library_url"`
			}
			rows := make([]row, 0, len(models))
			var upstream map[string]catalog.UpstreamMeta
			if fetchRelease {
				_ = cfg.EnsureDirs()
				cat := catalog.New(cfg.CacheDir, cfg.CacheTTL)
				names := make([]string, len(models))
				for i, m := range models {
					names[i] = m.Name
				}
				upstream = cat.UpstreamUpdatedBatch(ctx, names)
			}
			for _, m := range models {
				p := modelparse.Parse(m.Name, m.ParameterSize)
				r := row{Model: m, LibraryURL: p.LibraryURL(), Released: "â€”"}
				if meta, ok := upstream[m.Name]; ok && !meta.UpdatedAt.IsZero() {
					r.Released = meta.UpdatedAt.UTC().Format("2006-01-02")
				}
				rows = append(rows, r)
			}

			key := strings.ToLower(strings.TrimSpace(sortBy))
			switch key {
			case "", "name", "size", "params", "released":
			default:
				return fmt.Errorf("invalid --sort %q (use name|size|params|released)", sortBy)
			}
			if key == "" {
				key = "name"
			}
			sort.SliceStable(rows, func(i, j int) bool {
				cmp := 0
				switch key {
				case "size":
					switch {
					case rows[i].Size < rows[j].Size:
						cmp = -1
					case rows[i].Size > rows[j].Size:
						cmp = 1
					}
				case "params":
					pi, pj := parseParamBillions(rows[i].ParameterSize), parseParamBillions(rows[j].ParameterSize)
					switch {
					case pi < pj:
						cmp = -1
					case pi > pj:
						cmp = 1
					}
				case "released":
					ai, aj := rows[i].Released, rows[j].Released
					if ai == "â€”" {
						ai = "9999-99-99"
					}
					if aj == "â€”" {
						aj = "9999-99-99"
					}
					switch {
					case ai < aj:
						cmp = -1
					case ai > aj:
						cmp = 1
					}
				default: // name
					li, lj := strings.ToLower(rows[i].Name), strings.ToLower(rows[j].Name)
					switch {
					case li < lj:
						cmp = -1
					case li > lj:
						cmp = 1
					}
				}
				if cmp == 0 {
					return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
				}
				if sortDesc {
					return cmp > 0
				}
				return cmp < 0
			})

			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(rows)
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			if fetchRelease {
				fmt.Fprintln(w, "CTRY\tNAME\tSIZE\tPARAMS\tQUANT\tRELEASED\tDOWNLOADED\tLIBRARY")
			} else {
				fmt.Fprintln(w, "CTRY\tNAME\tSIZE\tPARAMS\tQUANT\tDOWNLOADED\tLIBRARY")
			}
			var total int64
			for _, r := range rows {
				p := modelparse.Parse(r.Name, r.ParameterSize)
				oi := origin.Lookup(p.BaseName)
				code := oi.Code
				if code == "" {
					code = "?"
				}
				// Single origin cell: emoji (terminals that support it) + ISO code — not repeated in NAME
				ctry := code
				if oi.Flag != "" && oi.Flag != "🏳️" {
					ctry = oi.Flag + " " + code
				}
				if fetchRelease {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						ctry,
						r.Name,
						ollama.FormatSize(r.Size),
						r.ParameterSize,
						r.QuantizationLevel,
						r.Released,
						r.ModifiedAt.Local().Format("2006-01-02"),
						r.LibraryURL,
					)
				} else {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						ctry,
						r.Name,
						ollama.FormatSize(r.Size),
						r.ParameterSize,
						r.QuantizationLevel,
						r.ModifiedAt.Local().Format("2006-01-02"),
						r.LibraryURL,
					)
				}
				total += r.Size
			}
			_ = w.Flush()
			fmt.Printf("\n%d models, %s total\n", len(models), ollama.FormatSize(total))
			if fetchRelease {
				fmt.Println("RELEASED = ollama.com library Updated date for that tag (upstream). DOWNLOADED = local modified_at.")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	c.Flags().BoolVar(&withRelease, "release", true, "include upstream library Updated (release) dates")
	c.Flags().BoolVar(&noRelease, "no-release", false, "skip upstream date lookup (faster, offline-friendly)")
	c.Flags().StringVar(&sortBy, "sort", "name", "sort by: name|size|params|released")
	c.Flags().BoolVar(&sortDesc, "desc", false, "sort descending")
	c.Flags().BoolVar(&byFamily, "family", false, "group by model family with size/feature pills")
	return c
}

func cmdCheck() *cobra.Command {
	var (
		asJSON    bool
		digest    bool
		notional  bool
		digestOff bool
		notionOff bool
	)
	c := &cobra.Command{
		Use:   "check",
		Short: "Check for same-tag updates and notional upgrades",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("digest") {
				digest = true
			}
			if !cmd.Flags().Changed("notional") {
				notional = true
			}
			if digestOff {
				digest = false
			}
			if notionOff {
				notional = false
			}

			ctx := context.Background()
			_ = cfg.EnsureDirs()
			cl := client()
			models, err := cl.List(ctx)
			if err != nil {
				return err
			}
			eng := &upgrade.Engine{
				Ollama:   cl,
				Registry: registry.New(),
				Catalog:  catalog.New(cfg.CacheDir, cfg.CacheTTL),
			}
			results := eng.CheckAll(ctx, models, upgrade.Options{
				CheckDigest:   digest,
				CheckNotional: notional,
				Pinned:        cfg.IsPinned,
				MaxCandidates: 3,
			})
			if asJSON {
				return json.NewEncoder(os.Stdout).Encode(results)
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSIZE\tSTATUS\tDETAIL")
			var nUpdate int
			for _, r := range results {
				detail := r.Message
				if len(r.Candidates) > 0 {
					detail = r.Message
				}
				status := strings.ToUpper(string(r.Kind))
				if r.Kind == upgrade.KindDigest || r.Kind == upgrade.KindNotional {
					nUpdate++
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Model, ollama.FormatSize(r.Size), status, detail)
			}
			_ = w.Flush()
			fmt.Printf("\n%d model(s) need attention\n", nUpdate)
			fmt.Println("Use: ollama-mgr upgrade <model> --to <target> --mode side-by-side|swap|pull")
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	c.Flags().BoolVar(&digest, "digest", true, "check same-tag registry digests")
	c.Flags().BoolVar(&notional, "notional", true, "check notional generation upgrades")
	c.Flags().BoolVar(&digestOff, "no-digest", false, "skip digest checks")
	c.Flags().BoolVar(&notionOff, "no-notional", false, "skip notional checks")
	return c
}

func cmdUpgrade() *cobra.Command {
	var (
		to   string
		mode string
		yes  bool
	)
	c := &cobra.Command{
		Use:   "upgrade <model>",
		Short: "Apply an upgrade: skip | side-by-side | swap | pull",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			from := args[0]
			m := actions.Mode(mode)
			if m == actions.ModeSwap && !yes {
				fmt.Fprintf(os.Stderr, "Swap will delete %s after pulling %s. Re-run with --yes to confirm.\n", from, to)
				return fmt.Errorf("confirmation required")
			}
			if (m == actions.ModeSideBySide || m == actions.ModeSwap) && to == "" {
				return fmt.Errorf("--to is required for %s", mode)
			}
			if m == actions.ModePull && to == "" {
				to = from
			}
			ctx := context.Background()
			cl := client()
			if m == actions.ModeSwap {
				fmt.Fprintf(os.Stderr, "Swap plan:\n  1) pull+verify %s\n  2) only then delete %s\n", to, from)
			}
			res, err := actions.ApplyUpgrade(ctx, cl, actions.UpgradeRequest{
				From: from,
				To:   to,
				Mode: m,
			}, func(ev actions.Event) {
				switch ev.Phase {
				case actions.PhasePulling:
					if ev.Pull != nil && ev.Pull.Total > 0 {
						printPullProgress(*ev.Pull)
					} else if ev.Message != "" {
						fmt.Fprintf(os.Stderr, "\r[%s] %s                    ", ev.Phase, ev.Message)
					}
				case actions.PhaseVerifying, actions.PhaseDeleting, actions.PhaseDone, actions.PhaseError, actions.PhaseQueued:
					fmt.Fprintf(os.Stderr, "\n[%s] %s\n", ev.Phase, ev.Message)
				default:
					if ev.Message != "" {
						fmt.Fprintf(os.Stderr, "\n[%s] %s\n", ev.Phase, ev.Message)
					}
				}
			})
			if err != nil {
				return err
			}
			fmt.Println(res.Message)
			if res.AlreadyHad && m == actions.ModeSwap {
				fmt.Println("(target was already installed â€” pull was quick; delete only ran after verify)")
			}
			return nil
		},
	}
	c.Flags().StringVar(&to, "to", "", "target model (successor or same)")
	c.Flags().StringVar(&mode, "mode", "side-by-side", "skip|side-by-side|swap|pull")
	c.Flags().BoolVar(&yes, "yes", false, "confirm destructive swap")
	return c
}

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
