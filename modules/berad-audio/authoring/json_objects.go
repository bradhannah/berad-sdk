package authoring

import (
	"bytes"
	"encoding/json"
	"strings"
)

// objectEntry locates one entry of a validated JSON object: start is its key's
// opening quote, valueStart and end bound its value.
type objectEntry struct {
	key        string
	start      int64
	valueStart int64
	end        int64
	value      []byte
}

// orderedEntries lists an object's entries in file order.
func orderedEntries(data []byte) []objectEntry {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if _, err := decoder.Token(); err != nil {
		panic(err)
	}
	var entries []objectEntry
	for decoder.More() {
		before := decoder.InputOffset()
		key, err := decoder.Token()
		if err != nil {
			panic(err)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			panic(err)
		}
		name, ok := key.(string)
		if !ok {
			panic("validated JSON object key is not a string")
		}
		end := decoder.InputOffset()
		start := before + int64(bytes.IndexByte(data[before:], '"'))
		entries = append(entries, objectEntry{key: name, start: start, valueStart: end - int64(len(raw)),
			end: end, value: raw})
	}

	return entries
}

// jsonIndent is how much deeper each level of a saved document is indented.
const jsonIndent = "  "

// lineIndent is the whitespace that begins the line holding offset.
func lineIndent(document []byte, offset int64) string {
	start := bytes.LastIndexByte(document[:offset], '\n') + 1
	line := document[start:offset]

	return string(line[:len(line)-len(bytes.TrimLeft(line, " \t"))])
}

// indentedObject lays an object's entries out one to a line, in key order,
// for an object whose line is indented by indent.
func indentedObject(indent string, entries map[string]json.RawMessage) string {
	lines := make([]string, 0, len(entries))
	for _, id := range sortedKeys(entries) {
		lines = append(lines, string(encodeAudioValue(id))+": "+string(entries[id]))
	}
	lead := "\n" + indent + jsonIndent

	return "{" + lead + strings.Join(lines, ","+lead) + "\n" + indent + "}"
}

// entryLead is the whitespace that begins an entry's line, after any comma.
func entryLead(data []byte, entries []objectEntry, index int) string {
	from := int64(1)
	if index > 0 {
		from = entries[index-1].end
	}
	between := string(data[from:entries[index].start])

	return between[strings.LastIndex(between, ",")+1:]
}

// rebuiltObject rewrites an object some entries left: kept entries keep their
// exact text, changed ones are re-encoded after their original key, new ones
// follow in key order, and the original spacing between entries is reused.
// An object that had no entries is laid out for a line indented by indent.
func rebuiltObject[T any](indent string, root jsonEdit, values map[string]T) jsonEdit {
	entries := orderedEntries(root.data)
	parts := make([]string, 0, len(values))
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seen[entry.key] = true
		if value, kept := values[entry.key]; kept {
			parts = append(parts, string(root.data[entry.start:entry.valueStart])+string(changedValue(entry.value, value)))
		}
	}
	for _, id := range sortedKeys(values) {
		if !seen[id] {
			parts = append(parts, string(encodeAudioValue(id))+": "+string(encodeAudioValue(values[id])))
		}
	}
	if len(parts) == 0 {
		return jsonEdit{start: root.start, end: root.end, data: []byte("{}")}
	}
	opening, closing := "\n"+indent+jsonIndent, "\n"+indent
	if len(entries) > 0 {
		opening = entryLead(root.data, entries, 0)
		closing = string(root.data[entries[len(entries)-1].end : len(root.data)-1])
	}
	separator := "," + opening
	if len(entries) > 1 {
		separator = string(root.data[entries[0].end:entries[1].start])
	}
	data := "{" + opening + strings.Join(parts, separator) + closing + "}"

	return jsonEdit{start: root.start, end: root.end, data: []byte(data)}
}

// changedValue keeps the original bytes of a value that did not change.
func changedValue[T any](original []byte, value T) []byte {
	var decoded T
	if err := json.Unmarshal(original, &decoded); err != nil {
		panic(err)
	}
	encoded := encodeAudioValue(value)
	if bytes.Equal(encoded, encodeAudioValue(decoded)) {
		return original
	}

	return encoded
}
