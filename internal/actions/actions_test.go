package actions

import (
	"context"
	"testing"
)

// Smoke tests that do not need a live Ollama daemon.

func TestSwapRejectsSameModel(t *testing.T) {
	_, err := ApplyUpgrade(context.Background(), nil, UpgradeRequest{
		From: "mistral:7b",
		To:   "mistral:7b",
		Mode: ModeSwap,
	}, nil)
	if err == nil {
		t.Fatal("expected error for same from/to")
	}
}

func TestSwapRequiresTo(t *testing.T) {
	_, err := ApplyUpgrade(context.Background(), nil, UpgradeRequest{
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
