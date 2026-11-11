package desktop

import (
	"embed"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Fonts are local and unmodified; licenses and pinned sources live beside them.
// Only a selected face is registered. Merely opening Settings loads no font.
//
//go:embed fonts/*.ttf fonts/*-OFL.txt
var fontFiles embed.FS

var editorFonts = []struct {
	id, name, family, file string
	once                   sync.Once
	err                    error
}{
	{id: "", name: "System monospace", family: "SF Mono, Menlo, monospace"},
	{id: "jetbrains-mono", name: "JetBrains Mono", family: "Yohaku JetBrains Mono", file: "JetBrainsMono-Regular.ttf"},
	{id: "ibm-plex-mono", name: "IBM Plex Mono", family: "Yohaku IBM Plex Mono", file: "IBMPlexMono-Regular.ttf"},
	{id: "source-code-pro", name: "Source Code Pro", family: "Yohaku Source Code Pro", file: "SourceCodePro-Regular.ttf"},
}

func editorFont(id string) (string, error) {
	for i := range editorFonts {
		f := &editorFonts[i]
		if f.id != id {
			continue
		}
		f.once.Do(func() {
			if f.file == "" {
				return
			}
			var data []byte
			data, f.err = fontFiles.ReadFile("fonts/" + f.file)
			if f.err == nil {
				f.err = ui.RegisterFont(data, f.family)
			}
		})
		if f.err != nil {
			return editorFonts[0].family, fmt.Errorf("%s: %w", f.name, f.err)
		}
		return f.family, nil
	}
	return editorFonts[0].family, fmt.Errorf("unknown editor font %q", id)
}
