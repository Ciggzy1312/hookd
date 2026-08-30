package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("ADDR=127.0.0.1:9090\n# comment\nFORWARD=\"http://127.0.0.1:9\"\n\nBAD\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if got["ADDR"] != "127.0.0.1:9090" || got["FORWARD"] != "http://127.0.0.1:9" {
		t.Fatalf("got %#v", got)
	}
	if _, ok := got["BAD"]; ok {
		t.Fatal("bare token should be skipped")
	}
	if Load(filepath.Join(dir, "missing")) != nil {
		t.Fatal("missing file should return nil")
	}
}
