package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kilrkrow/ollama-mgr/internal/catalog"
	"github.com/kilrkrow/ollama-mgr/internal/family"
	"github.com/kilrkrow/ollama-mgr/internal/modelparse"
	"github.com/kilrkrow/ollama-mgr/internal/ollama"
	"github.com/kilrkrow/ollama-mgr/internal/origin"
	"github.com/spf13/cobra"
)

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
