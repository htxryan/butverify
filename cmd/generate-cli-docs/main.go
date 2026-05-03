package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/htxryan/butverify/internal/cliref"
)

func main() {
	path := filepath.Clean(cliref.DocsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create docs dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, []byte(cliref.Markdown()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		os.Exit(1)
	}
}
