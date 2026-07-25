package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/guysc/ollama-mgr/internal/actions"
	"github.com/guysc/ollama-mgr/internal/ollama"
)

// Job is an in-flight or recently finished upgrade/pull operation.
type Job struct {
	ID        string       `json:"id"`
	From      string       `json:"from,omitempty"`
	To        string       `json:"to,omitempty"`
	Mode      actions.Mode `json:"mode"`
	Phase     actions.Phase `json:"phase"`
	Message   string       `json:"message"`
	Percent   float64      `json:"percent"` // 0-100 or -1
	Error     string       `json:"error,omitempty"`
	// UI staging hints
	PendingDelete bool `json:"pending_delete"` // tag the From row
	ShowDownload  bool `json:"show_download"`  // synthetic To row
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Done          bool      `json:"done"`
}

// Manager tracks async upgrade jobs for the GUI.
type Manager struct {
	mu   sync.RWMutex
	seq  int
	jobs map[string]*Job
}

// NewManager creates an empty job manager.
func NewManager() *Manager {
	return &Manager{jobs: map[string]*Job{}}
}

// List returns a snapshot of jobs (newest first), pruning very old finished ones.
func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	// prune done jobs older than 2 minutes
	cutoff := time.Now().Add(-2 * time.Minute)
	for id, j := range m.jobs {
		if j.Done && j.UpdatedAt.Before(cutoff) {
			delete(m.jobs, id)
		}
	}
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, *j)
	}
	// simple newest-first
	for i := 0; i < len(out); i++ {
		for k := i + 1; k < len(out); k++ {
			if out[k].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[k] = out[k], out[i]
			}
		}
	}
	return out
}

// Get returns a job by id.
func (m *Manager) Get(id string) (Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// StartUpgrade runs ApplyUpgrade asynchronously and tracks staged state.
func (m *Manager) StartUpgrade(client *ollama.Client, req actions.UpgradeRequest) string {
	m.mu.Lock()
	m.seq++
	id := time.Now().Format("150405") + "-" + itoa(m.seq)
	now := time.Now()
	j := &Job{
		ID:        id,
		From:      req.From,
		To:        req.To,
		Mode:      req.Mode,
		Phase:     actions.PhaseQueued,
		Message:   "queued",
		Percent:   -1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	// Stage UI immediately for swap / downloads
	switch req.Mode {
	case actions.ModeSwap:
		j.PendingDelete = true
		j.ShowDownload = true
		j.Message = "swap staged — will pull " + req.To + " then remove " + req.From
	case actions.ModeSideBySide, actions.ModePull:
		j.ShowDownload = true
		if req.To == "" {
			j.To = req.From
		}
		j.Message = "pull staged — " + j.To
	case actions.ModeSkip:
		j.Phase = actions.PhaseSkipped
		j.Message = "skipped"
		j.Done = true
	}
	m.jobs[id] = j
	m.mu.Unlock()

	if req.Mode == actions.ModeSkip {
		return id
	}

	go func() {
		ctx := context.Background()
		_, err := actions.ApplyUpgrade(ctx, client, req, func(ev actions.Event) {
			m.mu.Lock()
			defer m.mu.Unlock()
			jj, ok := m.jobs[id]
			if !ok {
				return
			}
			jj.Phase = ev.Phase
			jj.Message = ev.Message
			jj.Percent = ev.Percent
			jj.UpdatedAt = time.Now()
			if ev.To != "" {
				jj.To = ev.To
			}
			switch ev.Phase {
			case actions.PhasePulling, actions.PhaseVerifying:
				jj.ShowDownload = true
				if req.Mode == actions.ModeSwap {
					jj.PendingDelete = true
				}
			case actions.PhaseDeleting:
				jj.ShowDownload = true // target should be installed now
				jj.PendingDelete = true
			case actions.PhaseDone:
				jj.Done = true
				jj.PendingDelete = false
				jj.ShowDownload = false
				jj.Error = ""
			case actions.PhaseError:
				jj.Done = true
				jj.Error = ev.Message
				// Keep old model: clear pending delete
				jj.PendingDelete = false
				// Keep download row briefly if pull failed so user sees error on it
				if req.Mode == actions.ModeSwap {
					jj.ShowDownload = true
				}
			}
		})
		if err != nil {
			m.mu.Lock()
			if jj, ok := m.jobs[id]; ok {
				jj.Done = true
				jj.Phase = actions.PhaseError
				jj.Error = err.Error()
				jj.Message = err.Error()
				jj.PendingDelete = false
				jj.UpdatedAt = time.Now()
			}
			m.mu.Unlock()
		}
	}()

	return id
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
