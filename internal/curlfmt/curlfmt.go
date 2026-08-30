package curlfmt

import (
	"net/http"
	"sort"
	"strings"
)

// Command builds a POSIX curl one-liner for a captured request.
func Command(method, rawURL string, h http.Header, body []byte) string {
	var b strings.Builder
	b.WriteString("curl -X ")
	b.WriteString(quote(method))
	b.WriteByte(' ')
	b.WriteString(quote(rawURL))
	keys := make([]string, 0, len(h))
	for k := range h {
		if skip(k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range h[k] {
			b.WriteString(" -H ")
			b.WriteString(quote(k + ": " + v))
		}
	}
	if len(body) > 0 {
		b.WriteString(" --data-binary ")
		b.WriteString(quote(string(body)))
	}
	return b.String()
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func skip(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Content-Length", "Host", "Transfer-Encoding", "Connection", "Keep-Alive":
		return true
	default:
		return false
	}
}
