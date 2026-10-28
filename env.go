package main

import (
	"os"
	"strings"
)

// loadEnv sets the variables of the .env files at paths that the
// environment leaves unset, so that what the shell sets wins, then the
// first file, then the later ones. A file missing is skipped.
func loadEnv(paths ...string) {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for k, v := range parseEnv(string(b)) {
			if _, ok := os.LookupEnv(k); !ok {
				os.Setenv(k, v)
			}
		}
	}
}

// parseEnv reads a .env file: KEY=value lines, each maybe after export,
// with the value maybe in quotes. Blank lines and those starting with #
// are skipped, as is a # comment after a value out of quotes. A key set
// twice takes the later value.
func parseEnv(s string) map[string]string {
	vars := map[string]string{}
	for line := range strings.Lines(s) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		v = strings.TrimSpace(v)
		if n := len(v); n >= 2 && (v[0] == '"' || v[0] == '\'') && v[n-1] == v[0] {
			v = v[1 : n-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		vars[k] = v
	}
	return vars
}
