package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultRegistry = "https://registry.ollama.ai"

// Client fetches manifests from registry.ollama.ai.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a registry client.
func New() *Client {
	return &Client{
		BaseURL: defaultRegistry,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type manifest struct {
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"layers"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// ModelDigest returns the remote model weight digest for library-or-user path + tag.
// registryPath examples: "library/qwen2.5-coder", "user/model"
func (c *Client) ModelDigest(ctx context.Context, registryPath, tag string) (string, error) {
	if tag == "" {
		tag = "latest"
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.BaseURL, registryPath, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("not found in registry")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry %s: %s", resp.Status, truncate(body, 120))
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("invalid manifest: %w", err)
	}
	if len(m.Errors) > 0 {
		return "", fmt.Errorf("registry: %s", m.Errors[0].Message)
	}
	for _, layer := range m.Layers {
		if strings.Contains(layer.MediaType, "model") {
			return normalizeDigest(layer.Digest), nil
		}
	}
	return "", fmt.Errorf("no model layer in manifest")
}

// Exists reports whether a tag has a resolvable manifest.
func (c *Client) Exists(ctx context.Context, registryPath, tag string) bool {
	d, err := c.ModelDigest(ctx, registryPath, tag)
	return err == nil && d != ""
}

func normalizeDigest(d string) string {
	d = strings.TrimSpace(d)
	if strings.HasPrefix(d, "sha256:") {
		return d
	}
	if strings.HasPrefix(d, "sha256-") {
		return "sha256:" + strings.TrimPrefix(d, "sha256-")
	}
	if len(d) == 64 {
		return "sha256:" + d
	}
	return d
}

// DigestsEqual compares digests ignoring sha256- vs sha256: forms.
func DigestsEqual(a, b string) bool {
	return normalizeDigest(a) == normalizeDigest(b) && a != "" && b != ""
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
