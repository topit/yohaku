package notebook_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	"yohaku/internal/notebook"
)

func TestRichImagesCopyUndoArchiveAndRestore(t *testing.T) {
	b := notebook.NewBook()
	b.NewRich()
	n := b.Current()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.NRGBA{R: 250, A: 255})
	img.Set(2, 1, color.NRGBA{B: 190, A: 255})
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	for _, data := range []transfer.Data{transfer.TextData("开头😀\n"), transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, pngData.Bytes()))), transfer.TextData("\n结尾 {\\}")} {
		if err := n.PasteRich(data, false); err != nil {
			t.Fatal(err)
		}
	}
	want := n.Rich.Text().HTML()
	if !strings.Contains(want, "data:image/png;base64,") || !strings.Contains(want, "结尾") {
		t.Fatal(want)
	}
	data, err := notebook.RichClipboard(n.Rich.Text())
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := data.Read(transfer.Text)
	if string(plain) != "开头😀\n\n结尾 {\\}" {
		t.Fatalf("plain=%q", plain)
	}
	rtf, _ := data.Read("text/rtf")
	if !bytes.Contains(rtf, []byte(`\pngblip`)) || !bytes.Contains(rtf, []byte(`\u`)) {
		t.Fatal("RTF omitted image/Unicode")
	}
	// Both the external HTML representation and serialized Yohaku format
	// must retain real image bytes, not an in-process attachment pointer.
	for _, format := range []transfer.Format{transfer.HTML, notebook.RichFormat} {
		raw, _ := data.Read(format)
		b.NewRich()
		other := b.Current()
		if err := other.PasteRich(transfer.New(transfer.NewItem(transfer.Bytes(format, raw))), false); err != nil {
			t.Fatal(err)
		}
		if other.Rich.Text().HTML() != want {
			t.Fatal("cross-format copy lost content", other.Rich.Text().HTML(), want)
		}
		other.Rich.Undo()
		if other.Rich.Text().Len() != 0 {
			t.Fatal("paste not one undo step")
		}
		other.Rich.Redo()
		if other.Rich.Text().HTML() != want {
			t.Fatal("redo lost image")
		}
	}
	n = b.Current()
	n.Rich.SetSelection(1, 3)
	n.RichScroll = 14
	b.Close(b.Active)
	store := notebook.Store{Dir: t.TempDir()}
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	restored.Reopen()
	r := restored.Current()
	if r.Rich.Text().HTML() != want || r.RichScroll != 14 {
		t.Fatal("restart/archive lost image/scroll")
	}
	a, c := r.Rich.Selection()
	if a != 1 || c != 3 {
		t.Fatal("rune selection lost", a, c)
	}
	// A second save/reload catches serializers that decode an image but
	// forget its source bytes, then drop it on the next save.
	if err := store.Save(restored); err != nil {
		t.Fatal(err)
	}
	round2, err := store.Load()
	if err != nil || round2.Current().Rich.Text().HTML() != want {
		t.Fatal("second restart lost image", err)
	}
	before := n.Rich.Text().HTML()
	a, c = n.Rich.Selection()
	for _, html := range []string{`<p>partial<img src="https://example.com/x.png"></p>`, `<img src="data:image/png;base64,YmFk">`} {
		if err := n.PasteRich(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(html)))), false); err == nil {
			t.Fatal("unsafe image accepted")
		}
		if n.Rich.Text().HTML() != before {
			t.Fatal("failed paste partially changed text")
		}
		na, nc := n.Rich.Selection()
		if !reflect.DeepEqual([2]int{a, c}, [2]int{na, nc}) {
			t.Fatal("failed paste moved selection")
		}
	}
}

func TestImagePixelsAfterArchiveRestoreAndFirstDisplay(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 10))
	for y := range 10 {
		for x := range 16 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 15), G: uint8(y * 23), B: 190, A: uint8(64 + x*11)})
		}
	}
	for _, format := range []string{"png", "gif"} {
		t.Run(format, func(t *testing.T) {
			var encoded bytes.Buffer
			var err error
			if format == "png" {
				err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&encoded, img)
			} else {
				err = gif.Encode(&encoded, img, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			pixels, _, err := image.Decode(bytes.NewReader(encoded.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			b := notebook.NewBook()
			b.NewRich()
			url := "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
			html := []byte(`<p><img src="` + url + `"></p>`)
			if err := b.Current().PasteRich(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, html))), false); err != nil {
				t.Fatal(err)
			}
			b.Close(b.Active)
			store := notebook.Store{Dir: t.TempDir()}
			if err := store.Save(b); err != nil {
				t.Fatal(err)
			}
			restored, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			restored.Reopen()
			actual := restored.Current().Rich
			actual.SetSelection(0, 0)
			// Independent rendering reference: known decoded pixels, using the
			// framework's default attachment painter, not Yohaku's callback.
			reference := richtext.NewDocument(richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{
				Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: &richtext.Attachment{
					Image: ui.NewBitmap(pixels), Width: 16, Height: 10,
				}},
			}}}))
			reference.SetSelection(0, 0)
			render := func(d *richtext.Document) *image.RGBA {
				tt := ui.NewTester(func(c *ui.Context) { richtext.EditorBase(c, d).Fill() }, 160, 80)
				return tt.Image()
			}
			if !bytes.Equal(render(actual).Pix, render(reference).Pix) {
				t.Fatal("first display after archive/restart changed image pixels, alpha, or bounds")
			}
			data, err := notebook.RichClipboard(actual.Text())
			if err != nil {
				t.Fatal(err)
			}
			plainHTML, _ := data.Read(transfer.HTML)
			if !bytes.Contains(plainHTML, []byte("data:image/png;base64,")) {
				t.Fatal("copy lost embedded PNG after deferred display")
			}
		})
	}
}

func TestLargeImageRevisitCopyUndoAndRestart(t *testing.T) {
	b := notebook.NewBook()
	render := func(n *notebook.Note) *image.RGBA {
		n.Rich.SetSelection(0, 0)
		tt := ui.NewTester(func(c *ui.Context) { richtext.EditorBase(c, n.Rich).Fill() }, 600, 400)
		return tt.Image()
	}
	var first *notebook.Note
	var original, before []byte
	// Distinct large previews exceed the shared working set. Revisiting one
	// must recreate its display without reducing the copied original or undo.
	for i := range 12 {
		img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: uint8(i * 19), G: 120, B: 200, A: 255}), image.Point{}, draw.Src)
		var raw bytes.Buffer
		if err := png.Encode(&raw, img); err != nil {
			t.Fatal(err)
		}
		b.NewRich()
		if err := b.Current().PasteRich(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, raw.Bytes()))), false); err != nil {
			t.Fatal(err)
		}
		pixels := render(b.Current())
		if i == 0 {
			first, original, before = b.Current(), bytes.Clone(raw.Bytes()), bytes.Clone(pixels.Pix)
		}
	}
	check := func(n *notebook.Note) {
		t.Helper()
		if !bytes.Equal(before, render(n).Pix) {
			t.Fatal("revisit changed the preview")
		}
		data, err := notebook.RichClipboard(n.Rich.Text())
		if err != nil {
			t.Fatal(err)
		}
		html, _ := data.Read(transfer.HTML)
		if !bytes.Contains(html, []byte(base64.StdEncoding.EncodeToString(original))) {
			t.Fatal("copy replaced the full-resolution original with a thumbnail")
		}
	}
	check(first)
	first.Rich.Undo()
	if first.Rich.Text().Len() != 0 {
		t.Fatal("display eviction changed the paste undo step")
	}
	first.Rich.Redo()
	check(first)
	store := notebook.Store{Dir: t.TempDir()}
	if err := store.Save(b); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	check(loaded.Open[1])
}

func TestRestoreRejectsCorruptArchivedImageBeforeAnyDisplay(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 7, 5))); err != nil {
		t.Fatal(err)
	}
	truncated := encoded.Bytes()[:encoded.Len()-9]
	if _, _, err := image.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatal("fixture must pass header-only validation", err)
	}
	bad := richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement),
		Attributes: richtext.Attributes{Attachment: &richtext.Attachment{
			URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(truncated), Width: 7, Height: 5,
		}},
	}}})
	session := notebook.NewBook().Snapshot()
	session.Version = 2
	session.Closed = []notebook.SavedNote{{ID: "unvisited-archive", Title: "Broken archive",
		Text: notebook.Header("Plain Text"), Rich: &notebook.SavedRich{Text: bad}}}
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	store := notebook.Store{Dir: t.TempDir()}
	if err := os.WriteFile(store.Path(), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("corrupt unvisited archive accepted; full decoding must not be deferred")
	}
	after, err := os.ReadFile(store.Path())
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("failed restore modified the original session", err)
	}
}
