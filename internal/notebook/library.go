package notebook

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"
)

// The session is a small index. Note filenames are stable, independent of
// titles (including unnamed/duplicate titles), and cannot escape the library.
type librarySession struct {
	Version              int       `json:"version"`
	Open                 []noteRef `json:"open"`
	Closed               []noteRef `json:"closed"`
	Active               int       `json:"active"`
	HeynoteImportHandled bool      `json:"heynoteImportHandled,omitempty"`
}

type noteRef struct {
	ID         string             `json:"id"`
	Title      string             `json:"title"`
	File       string             `json:"file"`
	Selections []editor.Selection `json:"selections"`
	Scroll     float64            `json:"scroll"`
	Rich       *richPosition      `json:"rich,omitempty"`
}

type richPosition struct {
	Anchor int     `json:"anchor"`
	Caret  int     `json:"caret"`
	Scroll float64 `json:"scroll"`
}

// A stamp is installed on the UI thread after the complete transaction
// succeeds. Captured stamps are immutable and own no text/image copies.
type storedNote struct {
	dir     string
	version uint64
	rich    *richtext.Document
}

// SaveSnapshot contains only immutable source versions and copied metadata.
// Capture/Accept run on the UI thread; WriteSnapshot may run in one background
// writer. No worker reads the live Book, State, Document or image cache.
type SaveSnapshot struct {
	index librarySession
	notes []capturedNote
}

type capturedNote struct {
	ref      noteRef
	text     editor.Snapshot
	rich     richtext.Text
	document *richtext.Document // Identity only; never accessed by the writer.
	version  uint64
	stored   *storedNote
}

func (b *Book) CaptureSave() SaveSnapshot {
	s := SaveSnapshot{index: librarySession{Version: 3, Active: b.Active, HeynoteImportHandled: b.HeynoteImportHandled}}
	for _, group := range []struct {
		notes []*Note
		dst   *[]noteRef
	}{{b.Open, &s.index.Open}, {b.Closed, &s.index.Closed}} {
		for _, n := range group.notes {
			r := noteRef{ID: n.ID, Title: n.Title, File: noteFile(n.ID, n.Rich != nil), Selections: n.State.Selections(), Scroll: n.State.ScrollY()}
			if n.PendingScroll != nil {
				r.Scroll = *n.PendingScroll
			}
			c := capturedNote{ref: r, text: n.State.Buffer.Snapshot(), version: noteVersion(n), stored: n.stored, document: n.Rich}
			if n.Rich != nil {
				a, z := n.Rich.Selection()
				c.ref.Rich = &richPosition{a, z, n.RichScroll}
				c.rich = n.Rich.Text()
			}
			s.notes = append(s.notes, c)
			*group.dst = append(*group.dst, c.ref)
		}
	}
	return s
}

// Accept acknowledges exactly the versions written, never newer live edits.
// The caller invokes it only after a successful WriteSnapshot.
func (s SaveSnapshot) Accept(b *Book, store Store) {
	dir, _ := filepath.Abs(store.Dir)
	versions := make(map[string]storedNote, len(s.notes))
	for _, n := range s.notes {
		versions[n.ref.ID] = storedNote{dir, n.version, n.document}
	}
	for _, group := range [][]*Note{b.Open, b.Closed} {
		for _, n := range group {
			if v, ok := versions[n.ID]; ok {
				n.stored = &v
			}
		}
	}
}

func noteVersion(n *Note) uint64 {
	if n.Rich != nil {
		return n.Rich.Version()
	}
	return n.State.Buffer.Version()
}

func noteFile(id string, rich bool) string {
	name := fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
	if rich {
		return "notes/" + name + ".rich.json"
	}
	return "notes/" + name + ".txt"
}

func (s Store) NotesDir() string { return filepath.Join(s.Dir, "notes") }

func (s Store) checkDirectories(names ...string) error {
	for _, name := range names {
		info, err := os.Lstat(filepath.Join(s.Dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() { // Do not follow a nested symlink outside the library.
			return fmt.Errorf("library path is not a directory: %s", name)
		}
	}
	return nil
}

func (s Store) stamp(b *Book) {
	dir, _ := filepath.Abs(s.Dir)
	for _, group := range [][]*Note{b.Open, b.Closed} {
		for _, n := range group {
			n.stored = &storedNote{dir, noteVersion(n), n.Rich}
		}
	}
}

// Map only attachment URLs; all paragraph/run styles and image sizes survive.
// Clone runs/attachments so neither the live document nor undo is mutated.
func mapImages(t richtext.Text, mapURL func(string) (string, error)) (richtext.Text, error) {
	var ps []richtext.Paragraph
	for _, p := range t.Paragraphs() {
		p.Runs = slices.Clone(p.Runs)
		for i := range p.Runs {
			if a := p.Runs[i].Attributes.Attachment; a != nil {
				copy := *a
				url, err := mapURL(a.URL)
				if err != nil {
					return richtext.Text{}, err
				}
				copy.URL, copy.Draw = url, nil
				p.Runs[i].Attributes.Attachment = &copy
			}
		}
		ps = append(ps, p)
	}
	return richtext.TextOf(ps...), nil
}

func (s Store) loadLibrary(data []byte) (*Book, error) {
	if err := s.checkDirectories("notes", "images"); err != nil {
		return nil, err
	}
	var index librarySession
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	session := Session{Version: 2, Active: index.Active, HeynoteImportHandled: index.HeynoteImportHandled}
	for _, group := range []struct {
		refs []noteRef
		dst  *[]SavedNote
	}{{index.Open, &session.Open}, {index.Closed, &session.Closed}} {
		for _, r := range group.refs {
			if r.File != noteFile(r.ID, r.Rich != nil) {
				return nil, fmt.Errorf("note %q: invalid file reference", r.Title)
			}
			body, err := os.ReadFile(filepath.Join(s.Dir, r.File))
			if err != nil {
				return nil, fmt.Errorf("note %q (%s): %w", r.Title, r.File, err)
			}
			n := SavedNote{ID: r.ID, Title: r.Title, Text: string(body), Selections: r.Selections, Scroll: r.Scroll}
			if r.Rich != nil {
				var text richtext.Text
				if err := json.Unmarshal(body, &text); err != nil {
					return nil, fmt.Errorf("note %q: %w", r.Title, err)
				}
				text, err = mapImages(text, func(url string) (string, error) {
					if !validLibraryPath(url) || !strings.HasPrefix(url, "images/") {
						return "", errors.New("invalid image reference")
					}
					raw, err := os.ReadFile(filepath.Join(s.Dir, url))
					if err != nil {
						return "", err
					}
					if imageFile(raw) != url {
						return "", fmt.Errorf("image checksum mismatch: %s", url)
					}
					return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw), nil
				})
				if err != nil {
					return nil, fmt.Errorf("note %q: %w", r.Title, err)
				}
				n.Text = Header("Plain Text")
				n.Rich = &SavedRich{Text: text, Anchor: r.Rich.Anchor, Caret: r.Rich.Caret, Scroll: r.Rich.Scroll}
			}
			*group.dst = append(*group.dst, n)
		}
	}
	b, err := Restore(session) // Includes full pixel validation, even for archives.
	if err == nil {
		s.stamp(b)
	}
	return b, err
}

func imageFile(raw []byte) string { return fmt.Sprintf("images/%x.png", sha256.Sum256(raw)) }

func (s Store) saveLibrary(snapshot SaveSnapshot) error {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	if err := s.checkDirectories("notes", "images"); err != nil {
		return err
	}
	// Before first migration, preserve the exact legacy bytes, not a re-encoded
	// snapshot. The backup is never updated by subsequent saves.
	if old, err := os.ReadFile(s.Path()); err == nil {
		var header struct{ Version int }
		if json.Unmarshal(old, &header) == nil && (header.Version == 1 || header.Version == 2) {
			backup := filepath.Join(s.Dir, "session.legacy-backup.json")
			if info, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
				if err := AtomicWrite(backup, old); err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else if !info.Mode().IsRegular() {
				return fmt.Errorf("migration backup is not a file: %s", backup)
			} else {
				kept, err := os.ReadFile(backup)
				if err != nil {
					return err
				}
				if !bytes.Equal(kept, old) {
					return fmt.Errorf("legacy source changed since its backup; keep and rename %s before retrying migration", backup)
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tx, err := s.newSave()
	if err != nil {
		return err
	}
	// Before publication the staging directory is disposable. After publication
	// it must survive every error, including an uncertain directory-sync error.
	published := false
	defer func() {
		if !published {
			os.RemoveAll(filepath.Join(s.Dir, tx.Stage))
		}
	}()
	dir, _ := filepath.Abs(s.Dir)
	for _, n := range snapshot.notes {
		r := n.ref
		stored := n.stored
		_, exists := os.Stat(filepath.Join(s.Dir, r.File))
		if stored == nil || stored.dir != dir || stored.version != n.version || stored.rich != n.document || exists != nil {
			var body []byte
			if n.document == nil {
				body = []byte(n.text.String())
			} else {
				text, err := mapImages(n.rich, func(url string) (string, error) {
					encoded, ok := strings.CutPrefix(url, "data:image/png;base64,")
					if !ok {
						return "", errors.New("cannot save external image")
					}
					raw, err := base64.StdEncoding.DecodeString(encoded)
					if err != nil {
						return "", err
					}
					path := imageFile(raw)
					if _, err := os.Stat(filepath.Join(s.Dir, path)); errors.Is(err, os.ErrNotExist) {
						if err := tx.add(s, path, raw); err != nil {
							return "", err
						}
					} else if err != nil {
						return "", err
					}
					return path, nil
				})
				if err != nil {
					return err
				}
				body, err = json.Marshal(text)
				if err != nil {
					return err
				}
			}
			if err := tx.add(s, r.File, body); err != nil {
				return err
			}
		}
	}
	data, err := json.MarshalIndent(snapshot.index, "", "  ")
	if err != nil {
		return err
	}
	if len(tx.Files) == 0 {
		// A position/title/tab-only change replaces one file, so the index's
		// atomic rename is already a complete transaction; no replay is needed.
		if err := AtomicWrite(s.Path(), data); err != nil {
			return err
		}
		return nil
	}
	if err := tx.add(s, "session.json", data); err != nil {
		return err
	}
	marker, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	published = true
	if err := AtomicWrite(filepath.Join(s.Dir, pendingSave), marker); err != nil {
		return err
	}
	if err := s.recoverSave(); err != nil {
		return err
	}
	return nil
}

const pendingSave = ".pending-save.json"

type saveFile struct {
	Path string `json:"path"`
	Hash string `json:"sha256"`
}
type saveTransaction struct {
	Stage string     `json:"stage"`
	Files []saveFile `json:"files"`
}

func (s Store) newSave() (*saveTransaction, error) {
	name := ".save-" + newID()
	if err := os.Mkdir(filepath.Join(s.Dir, name), 0700); err != nil {
		return nil, err
	}
	return &saveTransaction{Stage: name}, nil
}

func (tx *saveTransaction) add(s Store, path string, data []byte) error {
	for _, f := range tx.Files {
		if f.Path == path { // Repeated images share a single staged resource.
			return nil
		}
	}
	if err := AtomicWrite(filepath.Join(s.Dir, tx.Stage, strconv.Itoa(len(tx.Files))), data); err != nil {
		return err
	}
	tx.Files = append(tx.Files, saveFile{path, fmt.Sprintf("%x", sha256.Sum256(data))})
	return nil
}

func hexName(s string, length int) bool {
	_, err := hex.DecodeString(s)
	return len(s) == length && err == nil && s == strings.ToLower(s)
}

func validLibraryPath(path string) bool {
	if path == "session.json" {
		return true
	}
	for _, form := range []struct{ prefix, suffix string }{{"notes/", ".txt"}, {"notes/", ".rich.json"}, {"images/", ".png"}} {
		if strings.HasPrefix(path, form.prefix) && strings.HasSuffix(path, form.suffix) {
			return hexName(strings.TrimSuffix(strings.TrimPrefix(path, form.prefix), form.suffix), 64)
		}
	}
	return false
}

// Publication is the commit point: from then on recovery rolls forward. Every
// staged payload is verified before any destination changes; replay is
// idempotent and the small index is always installed last. Removing + syncing
// the marker precedes deleting staging, including after a crash during cleanup.
func (s Store) recoverSave() error {
	marker := filepath.Join(s.Dir, pendingSave)
	data, err := os.ReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var tx saveTransaction
	if err := json.Unmarshal(data, &tx); err != nil {
		return err
	}
	if !strings.HasPrefix(tx.Stage, ".save-") || !hexName(strings.TrimPrefix(tx.Stage, ".save-"), 32) || len(tx.Files) == 0 || tx.Files[len(tx.Files)-1].Path != "session.json" {
		return errors.New("invalid pending save")
	}
	if err := s.checkDirectories(tx.Stage, "notes", "images"); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, f := range tx.Files {
		if !validLibraryPath(f.Path) || seen[f.Path] || !hexName(f.Hash, 64) {
			return errors.New("invalid pending file")
		}
		seen[f.Path] = true
		body, err := os.ReadFile(filepath.Join(s.Dir, tx.Stage, strconv.Itoa(i)))
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(body)) != f.Hash {
			return fmt.Errorf("pending save checksum mismatch: %s", f.Path)
		}
	}
	for i, f := range tx.Files {
		body, err := os.ReadFile(filepath.Join(s.Dir, tx.Stage, strconv.Itoa(i)))
		if err != nil {
			return err
		}
		if err := AtomicWrite(filepath.Join(s.Dir, f.Path), body); err != nil {
			return err
		}
	}
	if err := os.Remove(marker); err != nil {
		return err
	}
	if err := syncDirectory(s.Dir); err != nil {
		return err
	}
	// Failure to reclaim completed staging is not a failed content save.
	_ = os.RemoveAll(filepath.Join(s.Dir, tx.Stage))
	return nil
}
