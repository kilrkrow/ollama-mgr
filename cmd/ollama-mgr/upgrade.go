package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kilrkrow/ollama-mgr/internal/actions"
	"github.com/spf13/cobra"
)

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
