package notebook

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"
)

type SavedRich struct {
	Text   richtext.Text `json:"text"`
	Anchor int           `json:"anchor"`
	Caret  int           `json:"caret"`
	Scroll float64       `json:"scroll"`
}

type SavedNote struct {
	ID         string             `json:"id"`
	Title      string             `json:"title"`
	Text       string             `json:"text"`
	Selections []editor.Selection `json:"selections"`
	Scroll     float64            `json:"scroll"`
	Rich       *SavedRich         `json:"rich,omitempty"`
}
type Session struct {
	Version              int         `json:"version"`
	Open                 []SavedNote `json:"open"`
	Closed               []SavedNote `json:"closed"`
	Active               int         `json:"active"`
	HeynoteImportHandled bool        `json:"heynoteImportHandled,omitempty"`
}

func (b *Book) Snapshot() Session {
	s := Session{Version: 1, Active: b.Active, HeynoteImportHandled: b.HeynoteImportHandled}
	for _, group := range []struct {
		notes []*Note
		dst   *[]SavedNote
	}{{b.Open, &s.Open}, {b.Closed, &s.Closed}} {
		for _, n := range group.notes {
			scroll := n.State.ScrollY()
			if n.PendingScroll != nil {
				scroll = *n.PendingScroll
			}
			saved := SavedNote{ID: n.ID, Title: n.Title, Text: n.State.Buffer.String(), Selections: n.State.Selections(), Scroll: scroll}
			if n.Rich != nil {
				a, c := n.Rich.Selection()
				saved.Rich = &SavedRich{Text: n.Rich.Text(), Anchor: a, Caret: c, Scroll: n.RichScroll}
				s.Version = 2 // Older binaries must reject, not silently discard images.
			}
			*group.dst = append(*group.dst, saved)
		}
	}
	return s
}

func Restore(s Session) (*Book, error) {
	if s.Version != 1 && s.Version != 2 || len(s.Open) == 0 || s.Active < 0 || s.Active >= len(s.Open) {
		return nil, errors.New("invalid session version or active tab")
	}
	b := &Book{Active: s.Active, HeynoteImportHandled: s.HeynoteImportHandled}
	ids := make(map[string]bool)
	for _, group := range []struct {
		notes []SavedNote
		dst   *[]*Note
	}{{s.Open, &b.Open}, {s.Closed, &b.Closed}} {
		for _, saved := range group.notes {
			if saved.ID == "" || ids[saved.ID] || math.IsNaN(saved.Scroll) || math.IsInf(saved.Scroll, 0) || saved.Scroll < 0 {
				return nil, errors.New("invalid or duplicate note state")
			}
			ids[saved.ID] = true
			n, err := NewNote(saved.Title, saved.Text)
			if err != nil {
				return nil, fmt.Errorf("note %q: %w", saved.Title, err)
			}
			n.ID = saved.ID
			n.images = &b.images
			if r := saved.Rich; r != nil {
				if s.Version != 2 || r.Anchor < 0 || r.Caret < 0 || r.Anchor > r.Text.Len() || r.Caret > r.Text.Len() || r.Scroll < 0 || math.IsNaN(r.Scroll) || math.IsInf(r.Scroll, 0) {
					return nil, errors.New("invalid rich note state")
				}
				text, err := n.images.restore(r.Text)
				if err != nil {
					return nil, fmt.Errorf("note %q: %w", saved.Title, err)
				}
				n.Rich = richtext.NewDocument(text)
				n.Rich.SetSelection(r.Anchor, r.Caret)
				n.RichScroll = r.Scroll
			}
			for _, sel := range saved.Selections {
				if sel.Anchor < 0 || sel.Head < 0 || sel.Anchor > len(saved.Text) || sel.Head > len(saved.Text) {
					return nil, errors.New("invalid saved selection")
				}
				for _, at := range []int{sel.Anchor, sel.Head} {
					if at < len(saved.Text) && !utf8.RuneStart(saved.Text[at]) {
						return nil, errors.New("saved selection splits a UTF-8 character")
					}
				}
			}
			n.State.SetSelections(saved.Selections...)
			// Scroll is applied after the first layout; before layout it clamps
			// to zero. The desktop view owns this pending restoration.
			scroll := saved.Scroll
			n.PendingScroll = &scroll
			*group.dst = append(*group.dst, n)
		}
	}
	return b, nil
}

func DataDir() (string, error) {
	if runtime.GOOS == "linux" {
		if base := os.Getenv("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, "yohaku"), nil
		}
		home, err := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "yohaku"), err
	}
	base, err := os.UserConfigDir()
	return filepath.Join(base, "Yohaku"), err
}

type Store struct{ Dir string }

func (s Store) Path() string { return filepath.Join(s.Dir, "session.json") }
func (s Store) Load() (*Book, error) {
	if err := s.recoverSave(); err != nil {
		return nil, fmt.Errorf("cannot finish pending save (files kept): %w", err)
	}
	data, err := os.ReadFile(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		files, readErr := os.ReadDir(s.NotesDir())
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return nil, readErr
		}
		if len(files) > 0 {
			return nil, errors.New("session index is missing; existing note files kept, restore the complete library or a recovery export")
		}
		return NewBook(), nil
	}
	if err != nil {
		return nil, err
	}
	var header struct{ Version int }
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("cannot read %s (original kept): %w", s.Path(), err)
	}
	if header.Version == 3 {
		return s.loadLibrary(data)
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("cannot read %s (original kept): %w", s.Path(), err)
	}
	return Restore(session)
}
func (s Store) Save(b *Book) error {
	snapshot := b.CaptureSave()
	if err := s.WriteSnapshot(snapshot); err != nil {
		return err
	}
	snapshot.Accept(b, s)
	return nil
}

// WriteSnapshot must be serialized with all other writes to this library.
func (s Store) WriteSnapshot(snapshot SaveSnapshot) error {
	if err := s.recoverSave(); err != nil {
		return err
	}
	return s.saveLibrary(snapshot)
}

// AtomicWrite never truncates the last good file. Rename is same-directory;
// permissions are private, and both file data and the directory entry sync.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".yohaku-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func syncDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	} // directory fsync is not available here
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
