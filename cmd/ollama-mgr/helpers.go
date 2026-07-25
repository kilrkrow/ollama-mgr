package main

import (
	"strconv"
	"strings"

	"github.com/kilrkrow/ollama-mgr/internal/ollama"
)

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
