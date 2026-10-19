package editorui

import (
	"fmt"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

func (m *model) publicationDetails(add func(string, string)) {
	if m.doc.NeedsSync() {
		add("File durability: ", "Unconfirmed · Ctrl+s retries pending directory flushes")
	}
	receipt := m.doc.LastPublication()
	if receipt.State != audio.PublicationUnchanged && receipt.Path != "" {
		add("Last file operation: ", string(receipt.State))
		add("Operation path: ", receipt.Path)
	}
	if !receipt.DirectorySyncSupported {
		add("Platform limit: ", "Directory flush is unsupported; successful file publication is not full durability proof")
	}
	retained := m.doc.RetainedImports()
	if len(retained) != 0 {
		add("Retained imports: ", fmt.Sprintf("%d copied files remain independently of Undo", len(retained)))
	}
	for _, file := range retained {
		if !file.Referenced {
			add("Unassigned file: ", file.Path)
		}
	}
}
