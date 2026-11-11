package localplugin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yohaku/internal/localplugin"
)

func TestTypeScriptCommandsAreFreshAndHaveNoHostAPIs(t *testing.T) {
	dir, err := localplugin.CreateExample(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := localplugin.Inspect(dir)
	if err != nil {
		t.Fatal(err)
	}
	text, err := localplugin.Run(context.Background(), dir, m, m.Commands[0], localplugin.Input{Text: "中文  \nhello\t\n", Language: "Plain Text"})
	if err != nil || text != "中文\nhello\n" {
		t.Fatal(text, err)
	}
	source := `interface Input { readonly text: string }; let count: number = 0;
function identity<T>(x: T): T { return x; }
export function tidy(input: Input): string { count++; return identity<string>(input.text) + count + [typeof process,typeof require,typeof fetch,typeof setTimeout].join(","); }`
	if err = os.WriteFile(filepath.Join(dir, "main.ts"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		got, err := localplugin.Run(context.Background(), dir, m, m.Commands[0], localplugin.Input{Text: "go中文"})
		if err != nil || got != "go中文1undefined,undefined,undefined,undefined" {
			t.Fatal("TS erased or runtime persisted", got, err)
		}
	}
}

func TestPluginBoundsAndCompatibilityFailures(t *testing.T) {
	dir, _ := localplugin.CreateExample(t.TempDir())
	m, _ := localplugin.Inspect(dir)
	for _, tc := range []struct{ name, source, want string }{
		{"enum", "enum X {A}; export function tidy(): string {return 'x'}", "enum"},
		{"imports", `import {readFile} from "fs"; export function tidy(): string {return readFile('x')}`, "imports"},
		{"dynamic", `export function tidy(): string { return eval("'x'") }`, ""},
		{"promise", `export async function tidy(): Promise<string> { return 'x' }`, "synchronously"},
		{"throw", `export function tidy(): string { throw new Error('precise failure') }`, "precise failure"},
		{"output", `export function tidy(): string { return 'x'.repeat(1048577) }`, "at most 1 MiB"},
		{"budget", `export function tidy(): string { const keep: string[]=[]; const base='x'.repeat(65528);for(let i=0;i<2000;i++) keep.push(base+i);return keep.length.toString(); }`, "memory limit exceeded"},
		{"loop", `export function tidy(): string { while(true) {} }`, "deadline exceeded"},
		{"load-loop", `while(true) {} export function tidy(): string { return 'x' }`, "deadline exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, "main.ts"), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			// Output and memory cases must reach their own limits, rather than
			// racing a short cancellation timer on a loaded -race runner.
			limit := time.Second
			if tc.name == "loop" || tc.name == "load-loop" {
				limit = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), limit)
			defer cancel()
			start := time.Now()
			_, err := localplugin.Run(ctx, dir, m, m.Commands[0], localplugin.Input{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal(err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("execution did not stop")
			}
		})
	}
}

func TestPluginManifestCannotEscapeFolder(t *testing.T) {
	dir, _ := localplugin.CreateExample(t.TempDir())
	file := filepath.Join(dir, "yohaku-plugin.json")
	data, _ := os.ReadFile(file)
	bad := strings.Replace(string(data), `"main.ts"`, `"../main.ts"`, 1)
	if err := os.WriteFile(file, []byte(bad), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := localplugin.Inspect(dir); err == nil {
		t.Fatal("entry escaped plugin directory")
	}
}

func BenchmarkPluginInvocation(b *testing.B) {
	dir, err := localplugin.CreateExample(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	m, _ := localplugin.Inspect(dir)
	input := localplugin.Input{Text: strings.Repeat("hello 中文  \n", 100)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := localplugin.Run(context.Background(), dir, m, m.Commands[0], input); err != nil {
			b.Fatal(err)
		}
	}
}
