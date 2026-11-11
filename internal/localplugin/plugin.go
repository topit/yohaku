// Package localplugin runs explicitly trusted, offline text commands. A fresh
// MoeJS runtime belongs to each invocation; it never receives host capabilities.
package localplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MoeForge/moejs"
)

type Command struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Export string `json:"export"`
	Target string `json:"target"` // selection or block
}

type Manifest struct {
	APIVersion int       `json:"apiVersion"`
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Version    string    `json:"version"`
	Entry      string    `json:"entry"`
	Commands   []Command `json:"commands"`
}

type Input struct {
	Text      string `json:"text"`
	Language  string `json:"language"`
	NoteTitle string `json:"noteTitle"`
}

const MaxText = 1024 * 1024

var identifier = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

func read(root *os.Root, name string, limit int64) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("plugin file must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errors.New("plugin file exceeds size limit")
	}
	return data, err
}

// Inspect reads metadata only. Importing a directory does not execute its code.
func Inspect(dir string) (Manifest, error) {
	var m Manifest
	r, err := os.OpenRoot(dir)
	if err != nil {
		return m, err
	}
	defer r.Close()
	data, err := read(r, "yohaku-plugin.json", 16384)
	if err != nil {
		return m, err
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return m, err
	}
	if d.Decode(new(any)) != io.EOF {
		return m, errors.New("plugin manifest must contain one JSON object")
	}
	if m.APIVersion != 1 || !identifier.MatchString(m.ID) || strings.TrimSpace(m.Name) == "" || len(m.Name) > 80 || len(m.Version) > 40 || m.Version == "" ||
		filepath.Base(m.Entry) != m.Entry || !strings.HasSuffix(m.Entry, ".ts") || len(m.Commands) == 0 || len(m.Commands) > 16 {
		return m, errors.New("invalid plugin manifest (API 1, .ts entry and 1–16 commands required)")
	}
	seen := map[string]bool{}
	for _, c := range m.Commands {
		if !identifier.MatchString(c.ID) || !identifier.MatchString(c.Export) || strings.TrimSpace(c.Title) == "" || len(c.Title) > 80 || seen[c.ID] || (c.Target != "selection" && c.Target != "block") {
			return m, errors.New("invalid or duplicate plugin command")
		}
		seen[c.ID] = true
	}
	return m, nil
}

// Run compiles erasable TypeScript directly with the pinned MoeJS compiler.
// No npm, transpiler process, module resolver, timers or network APIs are added.
// The VM's memory accounting is a budget, not process-level memory isolation.
func Run(ctx context.Context, dir string, m Manifest, command Command, input Input) (string, error) {
	if len(input.Text) > MaxText {
		return "", errors.New("plugin input exceeds 1 MiB")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	r, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer r.Close()
	source, err := read(r, m.Entry, 65536)
	if err != nil {
		return "", err
	}
	mod, err := moejs.CompileWith(m.Entry, string(source), moejs.CompileOptions{TypeScript: true, Context: ctx})
	if err != nil {
		return "", err
	}
	hook, err := mod.Hook(command.Export)
	if err != nil {
		return "", err
	}
	vm := moejs.NewRuntime(moejs.Options{DisableDynamicCode: true, MemoryLimit: 32 * 1024 * 1024})
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()
	defer func() { close(done); <-stopped; vm.ClearInterrupt(); vm.ReleaseCallData() }()
	if err = vm.Load(mod); err != nil {
		return "", fmt.Errorf("plugin load: %w", err)
	}
	data, _ := json.Marshal(input)
	arg, err := vm.ParseJSONString(string(data))
	if err != nil {
		return "", err
	}
	v, err := vm.Call(hook, arg)
	if err != nil {
		return "", fmt.Errorf("plugin command: %w", err)
	}
	if !v.IsString() {
		return "", errors.New("plugin must return a string synchronously")
	}
	text := v.String()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if len(text) > MaxText || !utf8.ValidString(text) {
		return "", errors.New("plugin output must be valid UTF-8, at most 1 MiB")
	}
	return text, nil
}
