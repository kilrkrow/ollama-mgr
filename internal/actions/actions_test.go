package actions

import (
	"context"
	"fmt"
	"testing"

	"github.com/kilrkrow/ollama-mgr/internal/ollama"
)

type mockAPI struct {
	models  map[string]bool
	pulls   []string
	deletes []string
	failPull bool
	failDel  bool
}

func (m *mockAPI) List(ctx context.Context) ([]ollama.Model, error) {
	var out []ollama.Model
	for name := range m.models {
		out = append(out, ollama.Model{Name: name})
	}
	return out, nil
}

func (m *mockAPI) Pull(ctx context.Context, name string, onProgress func(ollama.PullProgress)) error {
	m.pulls = append(m.pulls, name)
	if m.failPull {
		return fmt.Errorf("pull failed")
	}
	if onProgress != nil {
		onProgress(ollama.PullProgress{Status: "success", Total: 100, Completed: 100})
	}
	m.models[name] = true
	return nil
}

func (m *mockAPI) Delete(ctx context.Context, name string) error {
	m.deletes = append(m.deletes, name)
	if m.failDel {
		return fmt.Errorf("delete failed")
	}
	delete(m.models, name)
	return nil
}

func TestSwapRejectsSameModel(t *testing.T) {
	_, err := ApplyUpgrade(context.Background(), &mockAPI{models: map[string]bool{}}, UpgradeRequest{
		From: "mistral:7b",
		To:   "mistral:7b",
		Mode: ModeSwap,
	}, nil)
	if err == nil {
		t.Fatal("expected error for same from/to")
	}
}

func TestSwapRequiresTo(t *testing.T) {
	_, err := ApplyUpgrade(context.Background(), &mockAPI{models: map[string]bool{}}, UpgradeRequest{
		From: "mistral:7b",
		Mode: ModeSwap,
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSkip(t *testing.T) {
	res, err := ApplyUpgrade(context.Background(), nil, UpgradeRequest{
		From: "mistral:7b",
		Mode: ModeSkip,
	}, nil)
	if err != nil || res.Message != "skipped" {
		t.Fatalf("skip: %+v %v", res, err)
	}
}

func TestSwapPullThenDeleteOrder(t *testing.T) {
	api := &mockAPI{models: map[string]bool{"old:7b": true}}
	var phases []Phase
	res, err := ApplyUpgrade(context.Background(), api, UpgradeRequest{
		From: "old:7b",
		To:   "new:7b",
		Mode: ModeSwap,
	}, func(e Event) { phases = append(phases, e.Phase) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != "old:7b" || res.Pulled != "new:7b" {
		t.Fatalf("result %+v", res)
	}
	if len(api.pulls) != 1 || api.pulls[0] != "new:7b" {
		t.Fatalf("pulls %v", api.pulls)
	}
	if len(api.deletes) != 1 || api.deletes[0] != "old:7b" {
		t.Fatalf("deletes %v", api.deletes)
	}
	// ensure delete only after pull succeeded: new present, old gone
	if api.models["old:7b"] || !api.models["new:7b"] {
		t.Fatalf("models state %+v", api.models)
	}
	// phase order includes pulling then deleting
	var sawPull, sawDel bool
	for _, p := range phases {
		if p == PhasePulling {
			sawPull = true
		}
		if p == PhaseDeleting {
			if !sawPull {
				t.Fatal("delete phase before pull")
			}
			sawDel = true
		}
	}
	if !sawDel {
		t.Fatal("no delete phase")
	}
}

func TestSwapKeepsOldOnPullFailure(t *testing.T) {
	api := &mockAPI{models: map[string]bool{"old:7b": true}, failPull: true}
	_, err := ApplyUpgrade(context.Background(), api, UpgradeRequest{
		From: "old:7b",
		To:   "new:7b",
		Mode: ModeSwap,
	}, nil)
	if err == nil {
		t.Fatal("expected pull error")
	}
	if len(api.deletes) != 0 {
		t.Fatalf("must not delete on pull fail: %v", api.deletes)
	}
	if !api.models["old:7b"] {
		t.Fatal("old model must remain")
	}
}

func TestSideBySideKeepsOld(t *testing.T) {
	api := &mockAPI{models: map[string]bool{"old:7b": true}}
	res, err := ApplyUpgrade(context.Background(), api, UpgradeRequest{
		From: "old:7b",
		To:   "new:7b",
		Mode: ModeSideBySide,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(api.deletes) != 0 {
		t.Fatal("side-by-side must not delete")
	}
	if !api.models["old:7b"] || !api.models["new:7b"] {
		t.Fatalf("both should exist: %+v", api.models)
	}
	if res.Pulled != "new:7b" {
		t.Fatalf("%+v", res)
	}
}
