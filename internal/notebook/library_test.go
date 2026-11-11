package notebook_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"
	"yohaku/internal/notebook"
)

func libraryImage(t *testing.T, b *notebook.Book) {
	t.Helper()
	b.NewRich()
	b.Rename("重复标题 😀")
	img := image.NewNRGBA(image.Rect(0, 0, 9, 5))
	img.Set(7, 3, color.NRGBA{R: 205, G: 19, A: 128})
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}
	for _, data := range []transfer.Data{transfer.TextData("图片前 中文😀\n"), transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, raw.Bytes()))), transfer.TextData("\n图片后")} {
		if err := b.Current().PasteRich(data, false); err != nil {
			t.Fatal(err)
		}
	}
}

func snapshotBytes(t *testing.T, b *notebook.Book) []byte {
	t.Helper()
	data, err := json.Marshal(b.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSaveSnapshotDoesNotAcknowledgeNewerLiveEdits(t *testing.T) {
	b := notebook.NewBook()
	b.Current().State.Insert("captured plain")
	libraryImage(t, b)
	want := snapshotBytes(t, b)
	snapshot := b.CaptureSave()
	store := notebook.Store{Dir: t.TempDir()}
	done := make(chan error, 1)
	go func() { done <- store.WriteSnapshot(snapshot) }()
	// Exercise immutable text, image/undo and metadata while the writer owns
	// the old snapshot. The race detector also checks the actual worker path.
	b.Current().Rich.Insert(richtext.NewText("newer rich"))
	b.Open[0].State.Insert(" newer plain")
	b.Rename("newer title")
	b.Close(0)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	snapshot.Accept(b, store)
	old, err := store.Load()
	if err != nil || !bytes.Equal(want, snapshotBytes(t, old)) {
		t.Fatal("background snapshot read live content", err)
	}
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Load()
	if err != nil || !bytes.Equal(snapshotBytes(t, b), snapshotBytes(t, latest)) {
		t.Fatal("old acknowledgment skipped newer note contents", err)
	}
}

func TestLibraryMigrationAndIncrementalSave(t *testing.T) {
	for _, rich := range []bool{false, true} {
		t.Run(map[bool]string{false: "v1", true: "v2-images"}[rich], func(t *testing.T) {
			b := notebook.NewBook()
			b.Current().State.Insert("未命名中文😀\nline two")
			first := b.Current().State.Buffer.String()
			b.New()
			b.Rename("重复标题 😀")
			b.Current().State.Insert("second / separate")
			if rich {
				libraryImage(t, b)
				libraryImage(t, b) // Same image in another archived document.
				b.Close(b.Active)
			}
			b.HeynoteImportHandled = true
			store := notebook.Store{Dir: t.TempDir()}
			legacy := append([]byte(" \n"), snapshotBytes(t, b)...)
			if err := os.WriteFile(store.Path(), legacy, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load()
			if err != nil || !bytes.Equal(snapshotBytes(t, loaded), snapshotBytes(t, b)) {
				t.Fatal("legacy restore", err)
			}
			// Block the backup, so migration must fail before touching any note.
			backup := filepath.Join(store.Dir, "session.legacy-backup.json")
			if err := os.Mkdir(backup, 0700); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(loaded); err == nil {
				t.Fatal("migration proceeded without a usable backup")
			}
			kept, _ := os.ReadFile(store.Path())
			if !bytes.Equal(kept, legacy) {
				t.Fatal("failed migration changed legacy source")
			}
			if err := os.Remove(backup); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(backup, []byte("older backup must not be overwritten"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(loaded); err == nil {
				t.Fatal("migration ignored a conflicting existing backup")
			}
			kept, _ = os.ReadFile(store.Path())
			if !bytes.Equal(kept, legacy) {
				t.Fatal("backup conflict overwrote the legacy source")
			}
			if err := os.Remove(backup); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(loaded); err != nil {
				t.Fatal(err)
			}
			kept, _ = os.ReadFile(backup)
			if !bytes.Equal(kept, legacy) {
				t.Fatal("backup did not preserve exact original bytes")
			}
			files, _ := os.ReadDir(store.NotesDir())
			infos := map[string]os.FileInfo{}
			foundText := false
			for _, f := range files {
				path := filepath.Join(store.NotesDir(), f.Name())
				infos[path], _ = os.Stat(path)
				raw, _ := os.ReadFile(path)
				foundText = foundText || string(raw) == first
				if bytes.Contains(raw, []byte("data:image")) {
					t.Fatal("image remained inline on disk")
				}
			}
			if !foundText || len(files) != len(b.Open)+len(b.Closed) {
				t.Fatal("independent readable notes missing")
			}
			if rich {
				images, _ := os.ReadDir(filepath.Join(store.Dir, "images"))
				if len(images) != 1 {
					t.Fatal("identical images were not deduplicated", len(images))
				}
				path := filepath.Join(store.Dir, "images", images[0].Name())
				infos[path], _ = os.Stat(path)
			}
			// Rename/archive/position changes must not rewrite any document/image.
			loaded.Rename("renamed / 不能改变文件名")
			loaded.Close(0)
			loaded.Reopen()
			if err := store.Save(loaded); err != nil {
				t.Fatal(err)
			}
			for path, before := range infos {
				after, err := os.Stat(path)
				if err != nil || !os.SameFile(before, after) {
					t.Fatal("metadata-only save rewrote document/image", path, err)
				}
			}
			loaded.Current().State.Insert("新增")
			if err := store.Save(loaded); err != nil {
				t.Fatal(err)
			}
			loaded.Current().State.Run(editor.Undo)
			if loaded.Current().State.Buffer.String() != first {
				t.Fatal("migration/save destroyed live undo")
			}
			loaded.Current().State.Run(editor.Redo)
			restored, err := store.Load()
			if err != nil || !bytes.Equal(snapshotBytes(t, restored), snapshotBytes(t, loaded)) {
				t.Fatal("v3 archive/image/position round trip", err)
			}
			kept, _ = os.ReadFile(backup)
			if !bytes.Equal(kept, legacy) {
				t.Fatal("subsequent save overwrote migration backup")
			}
		})
	}
}

func TestLibraryInterruptedSaveRecovery(t *testing.T) {
	for _, interruption := range []string{"partial-replace", "index-installed", "retry-newer", "bad-stage", "missing-stage", "path-traversal"} {
		t.Run(interruption, func(t *testing.T) {
			store := notebook.Store{Dir: t.TempDir()}
			b := notebook.NewBook()
			b.Current().State.Insert("first old 中文")
			b.New()
			b.Current().State.Insert("second old 😀")
			if err := store.Save(b); err != nil {
				t.Fatal(err)
			}
			var index struct{ Open []struct{ File string } }
			raw, _ := os.ReadFile(store.Path())
			if err := json.Unmarshal(raw, &index); err != nil {
				t.Fatal(err)
			}
			// First replacement succeeds; second is blocked after publication.
			blocked := filepath.Join(store.Dir, index.Open[1].File)
			if err := os.Remove(blocked); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(blocked, 0700); err != nil {
				t.Fatal(err)
			}
			for i, n := range b.Open {
				n.State.Insert(strings.Repeat("新增", i+1))
			}
			if err := store.Save(b); err == nil {
				t.Fatal("expected interrupted save")
			}
			if _, err := store.Load(); err == nil {
				t.Fatal("opened a mixed session while recovery remained blocked")
			}
			if err := os.Remove(blocked); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(store.Dir, ".pending-save.json")
			var tx struct {
				Stage string                          `json:"stage"`
				Files []struct{ Path, SHA256 string } `json:"files"`
			}
			pending, err := os.ReadFile(marker)
			if err != nil || json.Unmarshal(pending, &tx) != nil {
				t.Fatal("missing durable transaction", err)
			}
			switch interruption {
			case "bad-stage":
				if err := os.WriteFile(filepath.Join(store.Dir, tx.Stage, "0"), []byte("truncated"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-stage":
				if err := os.Remove(filepath.Join(store.Dir, tx.Stage, "1")); err != nil {
					t.Fatal(err)
				}
			case "path-traversal":
				tx.Files[0].Path = "../outside.txt"
				pending, _ = json.Marshal(tx)
				if err := os.WriteFile(marker, pending, 0600); err != nil {
					t.Fatal(err)
				}
			case "index-installed":
				// Simulate a crash after every destination, including the index,
				// is installed, but before the pending marker was removed.
				for i, f := range tx.Files {
					body, err := os.ReadFile(filepath.Join(store.Dir, tx.Stage, strconv.Itoa(i)))
					if err != nil {
						t.Fatal(err)
					}
					if err := notebook.AtomicWrite(filepath.Join(store.Dir, f.Path), body); err != nil {
						t.Fatal(err)
					}
				}
			case "retry-newer":
				b.Open[0].State.Insert("后续仍未保存的输入")
				if err := store.Save(b); err != nil {
					t.Fatal("retry did not finish pending then save newer content", err)
				}
			}
			restored, err := store.Load()
			if interruption == "bad-stage" || interruption == "missing-stage" || interruption == "path-traversal" {
				if err == nil {
					t.Fatal("corrupt transaction silently accepted")
				}
				after, _ := os.ReadFile(marker)
				if !bytes.Equal(after, pending) {
					t.Fatal("failed recovery discarded pending evidence")
				}
				return
			}
			if err != nil || !bytes.Equal(snapshotBytes(t, restored), snapshotBytes(t, b)) {
				t.Fatal("published save did not completely recover", err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("completed transaction marker remained", err)
			}
			if _, err := store.Load(); err != nil {
				t.Fatal("second restore was not idempotent", err)
			}
		})
	}
}

func TestLibraryRejectsMissingOrCorruptFiles(t *testing.T) {
	for _, damage := range []string{"index", "document", "image", "image-checksum", "image-directory-link", "note-traversal"} {
		t.Run(damage, func(t *testing.T) {
			b := notebook.NewBook()
			b.Current().State.Insert("保留，不允许静默新建")
			libraryImage(t, b)
			b.Close(b.Active) // Even an unvisited archived image must be checked.
			store := notebook.Store{Dir: t.TempDir()}
			if err := store.Save(b); err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(store.Path())
			var index struct{ Open []struct{ File string } }
			if err := json.Unmarshal(raw, &index); err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "index":
				if err := os.Remove(store.Path()); err != nil {
					t.Fatal(err)
				}
			case "document":
				if err := os.Remove(filepath.Join(store.Dir, index.Open[0].File)); err != nil {
					t.Fatal(err)
				}
			case "image", "image-checksum":
				files, _ := os.ReadDir(filepath.Join(store.Dir, "images"))
				path := filepath.Join(store.Dir, "images", files[0].Name())
				if damage == "image" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, []byte("broken PNG"), 0600); err != nil {
					t.Fatal(err)
				}
			case "note-traversal":
				raw = bytes.Replace(raw, []byte(index.Open[0].File), []byte("../outside.txt"), 1)
				if err := os.WriteFile(store.Path(), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "image-directory-link":
				kept := filepath.Join(t.TempDir(), "kept-images")
				if err := os.Rename(filepath.Join(store.Dir, "images"), kept); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(kept, filepath.Join(store.Dir, "images")); err != nil {
					t.Skip("symlinks unavailable", err)
				}
			}
			if _, err := store.Load(); err == nil {
				t.Fatal("damaged library silently accepted", damage)
			}
			if damage != "index" {
				after, _ := os.ReadFile(store.Path())
				if !bytes.Equal(after, raw) {
					t.Fatal("failed restore changed the index")
				}
			}
		})
	}
}

// Generate fixed inputs with scripts/perf-macos.py fixtures, then set
// YOHAKU_PERF_FIXTURES. Legacy is exactly the former MarshalIndent + AtomicWrite
// path. Allocations are cumulative per operation, not process footprint/peak.
func BenchmarkLibraryStorage(b *testing.B) {
	root := os.Getenv("YOHAKU_PERF_FIXTURES")
	if root == "" {
		b.Skip("set YOHAKU_PERF_FIXTURES to the fixed perf fixture root")
	}
	for _, scenario := range []string{"blank", "text10", "code30", "images", "text100", "image-heavy"} {
		if _, err := os.Stat(filepath.Join(root, scenario, "yohaku", "session.json")); os.IsNotExist(err) {
			continue
		}
		for _, format := range []string{"legacy", "library"} {
			for _, action := range []string{"Initial", "Metadata", "OneNote", "Restore"} {
				b.Run(scenario+"/"+format+"/"+action, func(b *testing.B) {
					raw, err := os.ReadFile(filepath.Join(root, scenario, "yohaku", "session.json"))
					if err != nil {
						b.Fatal(err)
					}
					var source notebook.Session
					if err := json.Unmarshal(raw, &source); err != nil {
						b.Fatal(err)
					}
					book, err := notebook.Restore(source)
					if err != nil {
						b.Fatal(err)
					}
					store := notebook.Store{Dir: filepath.Join(b.TempDir(), "data")}
					save := func() error {
						if format == "library" {
							return store.Save(book)
						}
						data, err := json.MarshalIndent(book.Snapshot(), "", "  ")
						if err != nil {
							return err
						}
						return notebook.AtomicWrite(store.Path(), data)
					}
					if err := save(); err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						switch action {
						case "Initial":
							if err := os.RemoveAll(store.Dir); err != nil {
								b.Fatal(err)
							}
						case "Metadata":
							book.Rename(strconv.Itoa(i))
						case "OneNote":
							book.Open[0].State.Insert("x")
						}
						b.StartTimer()
						if action == "Restore" {
							if format == "library" {
								_, err = store.Load()
							} else {
								var data []byte
								data, err = os.ReadFile(store.Path())
								if err == nil {
									var session notebook.Session
									err = json.Unmarshal(data, &session)
									if err == nil {
										_, err = notebook.Restore(session)
									}
								}
							}
						} else {
							err = save()
						}
						b.StopTimer()
						if err != nil {
							b.Fatal(err)
						}
						if action == "OneNote" {
							book.Open[0].State.Run(editor.Undo)
						}
						b.StartTimer()
					}
				})
			}
		}
	}
}
