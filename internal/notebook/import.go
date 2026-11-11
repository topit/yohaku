package notebook

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"yohaku/internal/blockspike"
)

// Heynote's text-file protocol, independently decoded; no Heynote runtime or
// implementation is bundled. A delimiter owns its preceding newline.
var heynoteHeader = regexp.MustCompile(`(?m)^∞∞∞([a-z]+)(-a)?(?:;[^\n]*)?\n`)

// ImportHeynote adds one new tab per file. All files must parse before any
// live state changes; existing tabs/history and every source file stay intact.
// Directories contribute non-hidden .txt files recursively, never symlinks.
func (b *Book) ImportHeynote(paths []string) (int, []string, error) {
	if len(paths) == 0 {
		return 0, nil, nil
	}
	files, err := heynoteFiles(paths)
	if err != nil {
		return 0, nil, err
	}
	if len(files) == 0 {
		return 0, nil, fmt.Errorf("no Heynote .txt files found")
	}
	var notes []*Note
	var warnings []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, nil, err
		}
		n, notices, err := parseHeynote(filepath.Base(path), string(data))
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		notes = append(notes, n)
		for _, notice := range notices {
			if !slices.Contains(warnings, notice) {
				warnings = append(warnings, notice)
			}
		}
	}
	b.Active = len(b.Open)
	b.Open = append(b.Open, notes...)
	b.Revision++
	return len(notes), warnings, nil
}

func heynoteFiles(paths []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	for _, path := range paths {
		root, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("not a regular file or directory: %s", path)
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type().IsRegular() && (!info.IsDir() || strings.EqualFold(filepath.Ext(path), ".txt")) && !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func parseHeynote(filename, data string) (*Note, []string, error) {
	if !utf8.ValidString(data) || strings.ContainsRune(data, 0) {
		return nil, nil, fmt.Errorf("expected UTF-8 text without NUL bytes")
	}
	data = strings.TrimPrefix(strings.ReplaceAll(data, "\r\n", "\n"), "\ufeff")
	first := 0
	if !strings.HasPrefix(data, "∞∞∞") {
		at := strings.Index(data, "\n∞∞∞")
		if at < 0 {
			return nil, nil, fmt.Errorf("no Heynote block delimiters found")
		}
		first = at + 1
	}
	var metadata struct{ FormatVersion, Name string }
	if prefix := strings.TrimSpace(data[:first]); prefix != "" {
		if err := json.Unmarshal([]byte(prefix), &metadata); err != nil {
			return nil, nil, fmt.Errorf("invalid Heynote metadata: %w", err)
		}
	}
	if metadata.FormatVersion != "" {
		parts := strings.Split(metadata.FormatVersion, ".")
		if len(parts) != 3 {
			return nil, nil, fmt.Errorf("unsupported Heynote format version %q", metadata.FormatVersion)
		}
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || i == 0 && n > 2 {
				return nil, nil, fmt.Errorf("unsupported Heynote format version %q", metadata.FormatVersion)
			}
		}
	}
	content := data[first:]
	headers := heynoteHeader.FindAllStringSubmatchIndex(content, -1)
	if len(headers) == 0 || headers[0][0] != 0 {
		return nil, nil, fmt.Errorf("invalid or incomplete first Heynote delimiter")
	}
	var out strings.Builder
	var warnings []string
	for i, h := range headers {
		end := len(content)
		if i+1 < len(headers) {
			end = headers[i+1][0] - 1
		}
		if end < h[1] {
			return nil, nil, fmt.Errorf("block %d has no separating newline", i+1)
		}
		body := content[h[1]:end]
		if strings.Contains(body, Prefix) {
			return nil, nil, fmt.Errorf("block %d contains Yohaku's reserved structural prefix; source unchanged", i+1)
		}
		token := content[h[2]:h[3]]
		language := blockspike.HeynoteLanguage(token)
		if language == "" {
			language = "Plain Text"
			warnings = append(warnings, fmt.Sprintf("Unsupported language %s kept as Plain Text.", token))
		}
		if strings.Contains(body, "<∞img;") {
			warnings = append(warnings, "Image references kept as text; attachments were not copied.")
		}
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(Header(language))
		out.WriteString(body)
	}
	title := strings.TrimSpace(metadata.Name)
	if title == "" {
		title = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	n, err := NewNote(title, out.String())
	return n, warnings, err
}
