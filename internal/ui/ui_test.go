package ui

import (
	"strings"
	"testing"
)

func TestTemplatesExecute(t *testing.T) {
	var b strings.Builder
	if err := Execute(&b, "landing", LandingData{InboxID: "abc", InboxURL: "http://127.0.0.1:8080/i/abc"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "http://127.0.0.1:8080/i/abc") {
		t.Fatal("landing missing url")
	}

	b.Reset()
	if err := Execute(&b, "inbox", InboxData{ID: "abc", URL: "http://127.0.0.1:8080/i/abc", ReplayURL: "http://127.0.0.1:9999/"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `data-inbox="abc"`) {
		t.Fatal("inbox missing data-inbox")
	}
	if !strings.Contains(b.String(), `/static/style.css`) {
		t.Fatal("inbox missing stylesheet")
	}
	if len(StyleCSS()) == 0 {
		t.Fatal("embedded stylesheet is empty")
	}

	b.Reset()
	if err := Execute(&b, "notfound", NotFoundData{ID: "nope"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "not found") {
		t.Fatal("notfound missing text")
	}
}
