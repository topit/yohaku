//go:build !darwin

package desktop

import (
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui/richtext"
)

func nativeRichClipboard(data transfer.Data, _ richtext.Text) (transfer.Data, error) {
	return data, nil
}
