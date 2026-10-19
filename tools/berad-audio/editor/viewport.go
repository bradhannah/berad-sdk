package editorui

// listViewportStart retains the visible page until selection crosses an edge.
// Paging explicitly moves both this origin and the selection, rather than
// letting a bottom-anchored cursor make the first page jump scroll one row.
func listViewportStart(start, cursor, count, height int) int {
	height = max(1, height)
	start = max(0, min(start, max(0, count-height)))
	if cursor < start {
		start = max(0, cursor)
	} else if cursor >= start+height {
		start = max(0, cursor-height+1)
	}

	return start
}

func (m *model) browserWindowStart() int {
	return listViewportStart(m.browserStart, m.cursor, len(m.keys), m.listHeight())
}
