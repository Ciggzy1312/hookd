package ui

import (
	"embed"
	"html/template"
	"io"
)

//go:embed *.html
var files embed.FS

//go:embed style.css
var styleCSS []byte

var pages = template.Must(template.ParseFS(files, "*.html"))

func StyleCSS() []byte { return styleCSS }

type LandingData struct {
	InboxID  string
	InboxURL string
}

type InboxData struct {
	ID  string
	URL string
}

type NotFoundData struct {
	ID string
}

func Execute(w io.Writer, name string, data any) error {
	return pages.ExecuteTemplate(w, name, data)
}
