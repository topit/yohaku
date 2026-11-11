package notebook

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
	xhtml "golang.org/x/net/html"
)

const RichFormat transfer.Format = "application/x-yohaku-rich-text+json"
const richLimit = 32 * 1024 * 1024

func (b *Book) NewRich() {
	b.New()
	b.Current().Title = "Image & text"
	b.Current().Rich = richtext.NewDocument(richtext.Text{})
}

// Embedded images make the atomic session/recovery copy self-contained. No
// filesystem references, downloads or attachment GC can invalidate undo/archive.
func (c *imageCache) attachment(raw []byte, name string) (*richtext.Attachment, error) {
	if len(raw) > 8*1024*1024 {
		return nil, errors.New("image exceeds 8 MiB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 12_000_000 {
		return nil, errors.New("invalid image or image exceeds 12 megapixels")
	}
	// Fully decode even unvisited/archived images. Header-only validation could
	// silently carry corrupt pixels into the next atomic save.
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if format != "png" {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			return nil, err
		}
		if encoded.Len() > 8*1024*1024 {
			return nil, errors.New("encoded image exceeds 8 MiB")
		}
		raw = encoded.Bytes()
	}
	w, h := float32(cfg.Width), float32(cfg.Height)
	scale := min(float32(1), 560/w, 320/h)
	// The immutable URL owns the original source. Shared, bounded display
	// bitmaps are separate from the document and its undo history.
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	key := sha256.Sum256(raw)
	return &richtext.Attachment{Name: name,
		URL:   url,
		Width: w * scale, Height: h * scale,
		Draw: func(p *ui.Painter, r ui.Rect) {
			p.Image(c.bitmap(key, url), r, ui.Contain)
		}}, nil
}

func (c *imageCache) attachmentURL(url, name string) (*richtext.Attachment, error) {
	header, encoded, ok := strings.Cut(url, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") || len(encoded) > 12*1024*1024 {
		return nil, errors.New("external/file images are not imported; paste the image itself")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid embedded image")
	}
	return c.attachment(raw, name)
}

// restore rebuilds immutable attachments, never mutating objects still
// held by another document or a historical undo step.
func (c *imageCache) restore(t richtext.Text) (richtext.Text, error) {
	var ps []richtext.Paragraph
	for _, p := range t.Paragraphs() {
		for i := range p.Runs {
			if old := p.Runs[i].Attributes.Attachment; old != nil {
				at, err := c.attachmentURL(old.URL, old.Name)
				if err != nil {
					return richtext.Text{}, err
				}
				if old.Width > 0 && old.Height > 0 && old.Width <= 4096 && old.Height <= 4096 && !math.IsNaN(float64(old.Width+old.Height)) {
					at.Width, at.Height = old.Width, old.Height
				}
				p.Runs[i].Attributes.Attachment = at
			}
		}
		ps = append(ps, p)
	}
	return richtext.TextOf(ps...), nil
}

func (cache *imageCache) parseHTML(source string) (richtext.Text, error) {
	root, err := xhtml.Parse(strings.NewReader(source))
	if err != nil {
		return richtext.Text{}, err
	}
	images := map[string]*richtext.Attachment{}
	var walk func(*xhtml.Node) error
	walk = func(n *xhtml.Node) error {
		if n.Type == xhtml.ElementNode && n.Data == "img" {
			var url, name string
			for _, a := range n.Attr {
				if a.Key == "src" {
					url = a.Val
				}
				if a.Key == "alt" {
					name = a.Val
				}
			}
			at, err := cache.attachmentURL(url, name)
			if err != nil {
				return err
			}
			key := "yohaku-image:" + strconv.Itoa(len(images))
			images[key] = at
			n.Attr = []xhtml.Attribute{{Key: "src", Val: key}, {Key: "alt", Val: name}}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return richtext.Text{}, err
	}
	var html bytes.Buffer
	if err := xhtml.Render(&html, root); err != nil {
		return richtext.Text{}, err
	}
	t := richtext.ParseHTML(html.String())
	var ps []richtext.Paragraph
	for _, p := range t.Paragraphs() {
		for i := range p.Runs {
			if at := p.Runs[i].Attributes.Attachment; at != nil {
				p.Runs[i].Attributes.Attachment = images[at.URL]
			}
		}
		ps = append(ps, p)
	}
	return richtext.TextOf(ps...), nil
}

// PasteRich validates the entire proposed paste before changing text, selection
// or undo. Its clipboard format preserves images even between processes.
func (n *Note) PasteRich(data transfer.Data, plain bool) error {
	var t richtext.Text
	var err error
	format, ok := data.Preferred(RichFormat, transfer.HTML, transfer.PNG, transfer.Text)
	if plain {
		format, ok = data.Preferred(transfer.Text)
	}
	if !ok {
		return errors.New("clipboard has no supported image or text")
	}
	raw, err := data.Read(format)
	if err != nil {
		return err
	}
	if len(raw) > richLimit {
		return errors.New("paste exceeds 32 MiB")
	}
	switch format {
	case RichFormat:
		if err = json.Unmarshal(raw, &t); err == nil {
			t, err = n.images.restore(t)
		}
	case transfer.HTML:
		t, err = n.images.parseHTML(string(raw))
	case transfer.PNG:
		var at *richtext.Attachment
		at, err = n.images.attachment(raw, "Pasted image")
		if err == nil {
			t = richtext.TextOf(richtext.Paragraph{Runs: []richtext.Run{{Text: string(richtext.ObjectReplacement), Attributes: richtext.Attributes{Attachment: at}}}})
		}
	case transfer.Text:
		t = richtext.NewText(string(raw))
	}
	if err != nil {
		return err
	}
	a, z := n.Rich.Selection()
	start, end := min(a, z), max(a, z)
	candidate := n.Rich.Text().Replace(start, end, t)
	encoded, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	if len(encoded) > richLimit {
		return errors.New("image & text note exceeds 32 MiB")
	}
	n.Rich.Edit(func(e *richtext.Edit) {
		e.Replace(start, end, t)
		e.Select(start+t.Len(), start+t.Len())
	})
	return nil
}

// RichClipboard offers alternatives of ONE ordered document, not separate
// clipboard items per image (which can lose the surrounding text at receivers).
func RichClipboard(t richtext.Text) (transfer.Data, error) {
	encoded, err := json.Marshal(t)
	if err != nil {
		return transfer.Data{}, err
	}
	var rtf strings.Builder
	rtf.WriteString(`{\rtf1\ansi\deff0{\fonttbl{\f0 Helvetica;}}\f0\fs28 `)
	for _, r := range t.Runs(0, t.Len()) {
		if at := r.Attributes.Attachment; at != nil {
			_, enc, ok := strings.Cut(at.URL, "data:image/png;base64,")
			if !ok {
				return transfer.Data{}, errors.New("image is not embedded")
			}
			png, err := base64.StdEncoding.DecodeString(enc)
			if err != nil {
				return transfer.Data{}, err
			}
			fmt.Fprintf(&rtf, `{\pict\pngblip\picwgoal%d\pichgoal%d %s}`, int(at.Width*15), int(at.Height*15), hex.EncodeToString(png))
			continue
		}
		rtf.WriteByte('{')
		if r.Attributes.Bold() {
			rtf.WriteString(`\b `)
		}
		if r.Attributes.Italic {
			rtf.WriteString(`\i `)
		}
		for _, c := range r.Text {
			switch c {
			case '\n':
				rtf.WriteString("\\par\n")
			case '\t':
				rtf.WriteString(`\tab `)
			case '\\', '{', '}':
				rtf.WriteByte('\\')
				rtf.WriteRune(c)
			default:
				if c >= 32 && c < 127 {
					rtf.WriteRune(c)
				} else {
					for _, u := range utf16.Encode([]rune{c}) {
						fmt.Fprintf(&rtf, `\u%d?`, int16(u))
					}
				}
			}
		}
		rtf.WriteByte('}')
	}
	rtf.WriteByte('}')
	return transfer.New(transfer.NewItem(transfer.Bytes(RichFormat, encoded),
		transfer.Bytes(transfer.HTML, []byte(t.HTML())), transfer.Bytes(richtext.RTF, []byte(rtf.String())),
		transfer.Bytes(transfer.Text, []byte(t.PlainText())))), nil
}
