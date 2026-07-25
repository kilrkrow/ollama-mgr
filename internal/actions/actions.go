package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/kilrkrow/ollama-mgr/internal/ollama"
)

// Mode is how to apply an upgrade.
type Mode string

const (
	ModeSkip       Mode = "skip"
	ModeSideBySide Mode = "side-by-side"
	ModeSwap       Mode = "swap"
	ModePull       Mode = "pull" // same-tag re-pull
)

// Phase is a high-level step in an upgrade job (for UI staging).
type Phase string

const (
	PhaseQueued    Phase = "queued"
	PhasePulling   Phase = "pulling"
	PhaseVerifying Phase = "verifying"
	PhaseDeleting  Phase = "deleting"
	PhaseDone      Phase = "done"
	PhaseError     Phase = "error"
	PhaseSkipped   Phase = "skipped"
)

// UpgradeRequest describes an upgrade operation.
type UpgradeRequest struct {
	From string // installed model (for swap delete)
	To   string // target to pull (successor or same)
	Mode Mode
}

// Event is a progress / phase update during ApplyUpgrade.
type Event struct {
	Phase   Phase                `json:"phase"`
	Message string               `json:"message"`
	From    string               `json:"from,omitempty"`
	To      string               `json:"to,omitempty"`
	Mode    Mode                 `json:"mode,omitempty"`
	Pull    *ollama.PullProgress `json:"pull,omitempty"`
	// Percent is 0-100 when known from pull totals; -1 if unknown.
	Percent float64 `json:"percent"`
}

// UpgradeResult is the outcome of ApplyUpgrade.
type UpgradeResult struct {
	Pulled  string `json:"pulled,omitempty"`
	Deleted string `json:"deleted,omitempty"`
	Message string `json:"message"`
	// AlreadyHad is true when the target was already fully present (pull was a no-op).
	AlreadyHad bool `json:"already_had,omitempty"`
}

// ModelAPI is the subset of ollama.Client used by upgrades (mockable in tests).
type ModelAPI interface {
	List(ctx context.Context) ([]ollama.Model, error)
	Pull(ctx context.Context, name string, onProgress func(ollama.PullProgress)) error
	Delete(ctx context.Context, name string) error
}

// ApplyUpgrade runs skip / side-by-side / swap / pull.
// onEvent is optional; used for staged UI (pending delete + download row).
//
// Swap safety: always pull+verify target BEFORE deleting the old model.
// If pull or verify fails, the old model is left untouched.
func ApplyUpgrade(ctx context.Context, client ModelAPI, req UpgradeRequest, onEvent func(Event)) (*UpgradeResult, error) {
	emit := func(e Event) {
		if onEvent != nil {
			e.From = req.From
			e.To = req.To
			e.Mode = req.Mode
			onEvent(e)
		}
	}

	switch req.Mode {
	case ModeSkip:
		emit(Event{Phase: PhaseSkipped, Message: "skipped", Percent: -1})
		return &UpgradeResult{Message: "skipped"}, nil

	case ModePull, ModeSideBySide, ModeSwap:
		target := req.To
		if target == "" {
			target = req.From
		}
		if req.Mode == ModeSwap {
			if req.To == "" {
				return nil, fmt.Errorf("swap requires --to target")
			}
			if req.From == "" {
				return nil, fmt.Errorf("swap requires source model")
			}
			if sameModel(req.From, req.To) {
				return nil, fmt.Errorf("swap from and to are the same model (%s)", req.From)
			}
		}
		if target == "" {
			return nil, fmt.Errorf("target model required")
		}
		if client == nil {
			return nil, fmt.Errorf("ollama client required")
		}

		// Snapshot: was target already installed?
		hadBefore, _ := modelPresent(ctx, client, target)

		emit(Event{
			Phase:   PhasePulling,
			Message: fmt.Sprintf("pulling %s...", target),
			To:      target,
			Percent: 0,
		})

		err := client.Pull(ctx, target, func(p ollama.PullProgress) {
			pct := -1.0
			if p.Total > 0 {
				pct = float64(p.Completed) / float64(p.Total) * 100
			}
			pp := p
			emit(Event{
				Phase:   PhasePulling,
				Message: pullStatusText(p),
				To:      target,
				Pull:    &pp,
				Percent: pct,
			})
		})
		if err != nil {
			emit(Event{Phase: PhaseError, Message: err.Error(), Percent: -1})
			return nil, fmt.Errorf("pull %s failed (old model kept): %w", target, err)
		}

		// Verify target is actually available locally before any delete.
		emit(Event{Phase: PhaseVerifying, Message: "verifying " + target, To: target, Percent: 100})
		ok, err := modelPresent(ctx, client, target)
		if err != nil {
			emit(Event{Phase: PhaseError, Message: err.Error(), Percent: -1})
			return nil, fmt.Errorf("verify after pull failed (old model kept): %w", err)
		}
		if !ok {
			msg := fmt.Sprintf("pull finished but %s is not in local model list (old model kept)", target)
			emit(Event{Phase: PhaseError, Message: msg, Percent: -1})
			return nil, fmt.Errorf("%s", msg)
		}

		res := &UpgradeResult{
			Pulled:     target,
			AlreadyHad: hadBefore,
		}

		if req.Mode == ModeSwap {
			emit(Event{
				Phase:   PhaseDeleting,
				Message: fmt.Sprintf("download OK - removing %s...", req.From),
				From:    req.From,
				To:      target,
				Percent: 100,
			})
			if err := client.Delete(ctx, req.From); err != nil {
				res.Message = fmt.Sprintf("pulled %s but failed to delete %s: %v", target, req.From, err)
				emit(Event{Phase: PhaseError, Message: res.Message, Percent: -1})
				return res, err
			}
			res.Deleted = req.From
			if hadBefore {
				res.Message = fmt.Sprintf("swapped: %s already present, removed %s", target, req.From)
			} else {
				res.Message = fmt.Sprintf("swapped: installed %s, removed %s", target, req.From)
			}
			emit(Event{Phase: PhaseDone, Message: res.Message, Percent: 100})
			return res, nil
		}

		// pull / side-by-side
		res.Message = "pulled " + target
		if req.Mode == ModeSideBySide && req.From != "" && !sameModel(req.From, target) {
			res.Message += " (kept " + req.From + ")"
		}
		if hadBefore {
			res.Message += " (already up to date / present)"
		}
		emit(Event{Phase: PhaseDone, Message: res.Message, Percent: 100})
		return res, nil

	default:
		return nil, fmt.Errorf("unknown mode %q (use skip|side-by-side|swap|pull)", req.Mode)
	}
}

func sameModel(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func modelPresent(ctx context.Context, client ModelAPI, name string) (bool, error) {
	models, err := client.List(ctx)
	if err != nil {
		return false, err
	}
	want := strings.ToLower(name)
	wantBase := want
	if strings.HasSuffix(want, ":latest") {
		wantBase = strings.TrimSuffix(want, ":latest")
	}
	for _, m := range models {
		n := strings.ToLower(m.Name)
		if n == want {
			return true, nil
		}
		if n == wantBase || n+":latest" == want {
			return true, nil
		}
	}
	return false, nil
}

func pullStatusText(p ollama.PullProgress) string {
	if p.Total > 0 {
		pct := float64(p.Completed) / float64(p.Total) * 100
		return fmt.Sprintf("%s %.0f%% (%s/%s)",
			p.Status,
			pct,
			ollama.FormatSize(p.Completed),
			ollama.FormatSize(p.Total),
		)
	}
	if p.Status != "" {
		return p.Status
	}
	return "pulling..."
}
