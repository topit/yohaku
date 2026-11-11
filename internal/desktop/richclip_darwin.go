package desktop

import (
	"bytes"
	"encoding/base64"
	"errors"
	"runtime"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui/richtext"
)

// AppKit's RTF reader ignores pngblip. Offer flat RTFD, with real attachment
// file wrappers, for native receivers. No Swift process, files or HTML loader.
func nativeRichClipboard(data transfer.Data, text richtext.Text) (transfer.Data, error) {
	msg := func(o objc.ID, name string, args ...any) objc.ID { return o.Send(objc.RegisterName(name), args...) }
	cls := func(name string) objc.ID { return objc.ID(objc.GetClass(name)) }
	pool := msg(msg(cls("NSAutoreleasePool"), "alloc"), "init")
	defer msg(pool, "drain")
	nsData := func(b []byte) objc.ID {
		d := msg(cls("NSData"), "dataWithBytes:length:", unsafe.Pointer(unsafe.SliceData(b)), uintptr(len(b)))
		runtime.KeepAlive(b)
		return d
	}
	nsString := func(s string) objc.ID {
		b := []byte(s)
		v := msg(msg(cls("NSString"), "alloc"), "initWithBytes:length:encoding:", unsafe.Pointer(unsafe.SliceData(b)), uintptr(len(b)), uintptr(4))
		runtime.KeepAlive(b)
		return msg(v, "autorelease")
	}
	doc := msg(msg(cls("NSMutableAttributedString"), "alloc"), "init")
	defer msg(doc, "release")
	addr, err := purego.Dlsym(purego.RTLD_DEFAULT, "objc_msgSend")
	if err != nil {
		return transfer.Data{}, err
	}
	var setSize func(objc.ID, objc.SEL, struct{ W, H float64 })
	purego.RegisterFunc(&setSize, addr)
	for _, run := range text.Runs(0, text.Len()) {
		var part objc.ID
		if at := run.Attributes.Attachment; at != nil {
			encoded := strings.TrimPrefix(at.URL, "data:image/png;base64,")
			raw, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return transfer.Data{}, err
			}
			// RTFD derives physical dimensions from the file, not attachment
			// bounds. Write PNG resolution metadata without resampling pixels.
			bitmap := msg(cls("NSBitmapImageRep"), "imageRepWithData:", nsData(raw))
			setSize(bitmap, objc.RegisterName("setSize:"), struct{ W, H float64 }{float64(at.Width), float64(at.Height)})
			png := msg(bitmap, "representationUsingType:properties:", uintptr(4), msg(cls("NSDictionary"), "dictionary"))
			if png == 0 {
				return transfer.Data{}, errors.New("AppKit could not encode image attachment")
			}
			wrapper := msg(msg(cls("NSFileWrapper"), "alloc"), "initRegularFileWithContents:", png)
			msg(wrapper, "setPreferredFilename:", nsString("image.png"))
			attachment := msg(msg(cls("NSTextAttachment"), "alloc"), "initWithFileWrapper:", wrapper)
			part = msg(cls("NSAttributedString"), "attributedStringWithAttachment:", attachment)
			msg(attachment, "release")
			msg(wrapper, "release")
		} else {
			fontSelector := "systemFontOfSize:"
			if run.Attributes.Bold() {
				fontSelector = "boldSystemFontOfSize:"
			}
			font := msg(cls("NSFont"), fontSelector, float64(15))
			attrs := msg(cls("NSDictionary"), "dictionaryWithObject:forKey:", font, nsString("NSFont"))
			part = msg(msg(cls("NSAttributedString"), "alloc"), "initWithString:attributes:", nsString(run.Text), attrs)
			msg(part, "autorelease")
		}
		msg(doc, "appendAttributedString:", part)
	}
	var rtfd func(objc.ID, objc.SEL, struct{ Location, Length uintptr }, objc.ID) objc.ID
	purego.RegisterFunc(&rtfd, addr)
	encoded := rtfd(doc, objc.RegisterName("RTFDFromRange:documentAttributes:"), struct{ Location, Length uintptr }{0, uintptr(msg(doc, "length"))}, 0)
	if encoded == 0 {
		return transfer.Data{}, errors.New("AppKit could not encode image & text")
	}
	n := uintptr(msg(encoded, "length"))
	ptr := objc.Send[unsafe.Pointer](encoded, objc.RegisterName("bytes"))
	raw := bytes.Clone(unsafe.Slice((*byte)(ptr), n))
	var reps []transfer.Representation
	for _, f := range data.Formats() {
		b, err := data.Read(f)
		if err != nil {
			return transfer.Data{}, err
		}
		reps = append(reps, transfer.Bytes(f, b))
	}
	reps = append(reps, transfer.Bytes("application/rtfd", raw))
	return transfer.New(transfer.NewItem(reps...)), nil
}
