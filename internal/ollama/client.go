package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Client talks to a local (or remote) Ollama HTTP API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a client for the given base URL (no trailing slash).
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Model is a locally available model summary.
type Model struct {
	Name              string    `json:"name"`
	Model             string    `json:"model"`
	ModifiedAt        time.Time `json:"modified_at"`
	Size              int64     `json:"size"`
	Digest            string    `json:"digest"`
	Details           Details   `json:"details"`
	Capabilities      []string  `json:"capabilities,omitempty"`
	ParameterSize     string    `json:"parameter_size,omitempty"`     // convenience from Details
	QuantizationLevel string    `json:"quantization_level,omitempty"` // convenience from Details
	ContextLength     int64     `json:"context_length,omitempty"`     // convenience from Details
}

// Details holds model format metadata.
type Details struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
	ContextLength     int64    `json:"context_length"`
	EmbeddingLength   int64    `json:"embedding_length"`
}

type tagsResponse struct {
	Models []Model `json:"models"`
}

// List returns installed models.
func (c *Client) List(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama unreachable at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list models: %s: %s", resp.Status, truncate(body, 200))
	}
	var tr tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	for i := range tr.Models {
		tr.Models[i].ParameterSize = tr.Models[i].Details.ParameterSize
		tr.Models[i].QuantizationLevel = tr.Models[i].Details.QuantizationLevel
		tr.Models[i].ContextLength = tr.Models[i].Details.ContextLength
		if tr.Models[i].Name == "" {
			tr.Models[i].Name = tr.Models[i].Model
		}
	}
	return tr.Models, nil
}

// ShowResponse is a subset of /api/show.
type ShowResponse struct {
	Modelfile  string          `json:"modelfile"`
	Parameters string          `json:"parameters"`
	Template   string          `json:"template"`
	Details    Details         `json:"details"`
	ModelInfo  json.RawMessage `json:"model_info"`
}

// Show returns details for a model.
func (c *Client) Show(ctx context.Context, name string) (*ShowResponse, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("show %s: %s: %s", name, resp.Status, truncate(b, 200))
	}
	var sr ShowResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

// LocalWeightDigest extracts the model weight blob digest from show modelfile.
// Returns form "sha256:hex" or empty if unknown.
func LocalWeightDigest(show *ShowResponse) string {
	if show == nil {
		return ""
	}
	// Modelfile lines like: FROM sha256:abc... or # sha256-abc in some versions
	mf := show.Modelfile
	for _, line := range strings.Split(mf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			rest := strings.TrimSpace(line[5:])
			if strings.HasPrefix(rest, "sha256:") {
				return rest
			}
			if strings.HasPrefix(rest, "sha256-") {
				return "sha256:" + strings.TrimPrefix(rest, "sha256-")
			}
		}
	}
	// fallback: search any sha256- hex
	const marker = "sha256-"
	if i := strings.Index(mf, marker); i >= 0 {
		hex := mf[i+len(marker):]
		end := 0
		for end < len(hex) && isHex(hex[end]) {
			end++
		}
		if end >= 40 {
			return "sha256:" + hex[:end]
		}
	}
	return ""
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// Delete removes a local model.
func (c *Client) Delete(ctx context.Context, name string) error {
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// delete can take a while on large models
	client := *c.HTTPClient
	client.Timeout = 10 * time.Minute
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete %s: %s: %s", name, resp.Status, truncate(b, 200))
	}
	return nil
}

// PullProgress is a streaming pull status update.
type PullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

// Pull downloads or updates a model, calling onProgress for each stream line.
func (c *Client) Pull(ctx context.Context, name string, onProgress func(PullProgress)) error {
	body, _ := json.Marshal(map[string]any{"name": name, "stream": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := *c.HTTPClient
	client.Timeout = 0 // no overall timeout for large pulls
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull %s: %s: %s", name, resp.Status, truncate(b, 200))
	}
	sc := bufio.NewScanner(resp.Body)
	// large JSON lines possible
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		var p PullProgress
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			continue
		}
		if p.Error != "" {
			return fmt.Errorf("pull %s: %s", name, p.Error)
		}
		if onProgress != nil {
			onProgress(p)
		}
	}
	return sc.Err()
}

// RunningModel is a model currently loaded in VRAM.
type RunningModel struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	SizeVRAM  int64     `json:"size_vram"`
	ExpiresAt time.Time `json:"expires_at"`
}

type psResponse struct {
	Models []RunningModel `json:"models"`
}

// Ps lists running models.
func (c *Client) Ps(ctx context.Context) ([]RunningModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ps: %s: %s", resp.Status, truncate(b, 200))
	}
	var pr psResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	return pr.Models, nil
}

// Ping checks whether the daemon responds.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}

// RunInteractive launches `ollama run <model>` attached to the current console.
func RunInteractive(model string) error {
	cmd := exec.Command("ollama", "run", model)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// StartServe starts `ollama serve` in the background (detached).
func StartServe() error {
	cmd := exec.Command("ollama", "serve")
	// detach on Windows: don't wait
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ollama serve: %w", err)
	}
	// give it a moment
	time.Sleep(800 * time.Millisecond)
	return nil
}

// FormatSize formats bytes for display.
func FormatSize(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "â€¦"
	}
	return s
}
