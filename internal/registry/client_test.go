package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelDigestAndEqual(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"layers": [
				{"mediaType": "application/vnd.ollama.image.model", "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "size": 1}
			]
		}`))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	d, err := c.ModelDigest(context.Background(), "library/mistral", "7b")
	if err != nil {
		t.Fatal(err)
	}
	want := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if d != want {
		t.Fatalf("digest=%s", d)
	}
	if !DigestsEqual(want, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatal("equal failed")
	}
}
