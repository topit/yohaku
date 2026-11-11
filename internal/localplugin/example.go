package localplugin

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed example/*
var example embed.FS

// CreateExample writes into a new directory, never overwriting user sources.
func CreateExample(parent string) (string, error) {
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(parent, "tidy-text-")
	if err != nil {
		return "", err
	}
	for _, name := range []string{"yohaku-plugin.json", "main.ts", "yohaku.d.ts"} {
		data, _ := example.ReadFile("example/" + name)
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}
