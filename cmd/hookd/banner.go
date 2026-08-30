package main

import (
	"fmt"
	"io"
	"os"
)

func colorOK() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func writeBanner(w io.Writer, baseURL, inboxURL, forward string) {
	name := "hookd"
	if colorOK() {
		name = "\x1b[32mhookd\x1b[0m"
	}
	fmt.Fprintf(w, "%s  %s\n  inbox %s\n", name, baseURL, inboxURL)
	if forward != "" {
		fmt.Fprintf(w, "  forward %s\n", forward)
	}
}
