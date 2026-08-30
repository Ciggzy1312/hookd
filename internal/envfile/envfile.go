package envfile

import (
	"os"
	"strings"
)

// Load reads KEY=VALUE lines. Empty lines and # comments are ignored.
func Load(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := make(map[string]string)
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" {
			out[k] = v
		}
	}
	return out
}
