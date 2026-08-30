package curlfmt

import (
	"net/http"
	"strings"
	"testing"
)

func TestCommand(t *testing.T) {
	got := Command(http.MethodPost, "/i/abc?x=1", http.Header{
		"Content-Type":   []string{"application/json"},
		"X-Test-Hook":    []string{"stripe"},
		"Content-Length": []string{"8"},
	}, []byte(`{"hi":1}`))
	if !strings.Contains(got, "curl -X 'POST' '/i/abc?x=1'") {
		t.Fatalf("method/url: %s", got)
	}
	if !strings.Contains(got, `-H 'Content-Type: application/json'`) || !strings.Contains(got, `-H 'X-Test-Hook: stripe'`) {
		t.Fatalf("headers: %s", got)
	}
	if !strings.Contains(got, `--data-binary '{"hi":1}'`) {
		t.Fatalf("body: %s", got)
	}
	if strings.Contains(got, "Content-Length") {
		t.Fatal("should skip Content-Length")
	}
}

func TestCommandQuotes(t *testing.T) {
	got := Command(http.MethodPost, "/i/x", nil, []byte(`it's`))
	if !strings.Contains(got, `'it'"'"'s'`) {
		t.Fatalf("quote: %s", got)
	}
}
