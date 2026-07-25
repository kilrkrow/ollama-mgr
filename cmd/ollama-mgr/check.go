package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/kilrkrow/ollama-mgr/internal/catalog"
	"github.com/kilrkrow/ollama-mgr/internal/ollama"
	"github.com/kilrkrow/ollama-mgr/internal/registry"
	"github.com/kilrkrow/ollama-mgr/internal/upgrade"
	"github.com/spf13/cobra"
)

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
