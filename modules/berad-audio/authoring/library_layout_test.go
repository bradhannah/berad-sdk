package authoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// grownLibrary is the layout a library created empty keeps as a recording,
// a cue and its ambient settings are added: each entry on its own line,
// indented one level deeper than the object holding it.
const grownLibrary = `{
  "schemaVersion": 5,
  "recordings": {
    "hum": {"file":null,"provenance":{"originalFilename":"unknown","sourceType":"unknown",` +
	`"sourceUrl":"unknown","sourceCreator":"unknown","license":"unknown","importedFrom":"unknown",` +
	`"processing":[],"commercialUse":false}}
  },
  "cues": {
    "ambient/hum": {"name":"Hum","recordingId":"hum","volumeModifier":1,"howToHear":"A low hum"}
  },
  "taxonomy": {
    "recordingFamilies": {
      "unreviewed": ["hum"]
    },
    "cueProfiles": {
      "unclassified": ["ambient/hum"]
    },
    "profiles": {
      "unclassified": {"targetAcousticFamily":"unknown","phase":"unknown","outcome":"unknown",` +
	`"mixRole":"unknown","emitters":["unknown"],"contexts":["unknown"]}
    }
  },
  "routing": {
    "ambient": {
      "ambient/hum": {"radiusTiles":8,"fadeInMs":800,"fadeOutMs":1200}
    }
  }
}
`

// TestAddedEntriesKeepTheLayout grows an empty library and checks that what
// is added inside the taxonomy and routing is laid out like the rest, and
// that a routing section the file lacks is added rather than routing being
// rewritten.
func TestAddedEntriesKeepTheLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	require.NoError(t, CreateLibrary(path))
	editor, err := OpenLibraryEditor(path)
	require.NoError(t, err)
	require.NoError(t, editor.AddRecording("hum"))
	require.NoError(t, editor.CreateCue("ambient/hum", Cue{Name: "Hum", RecordingID: "hum", VolumeModifier: 1,
		HowToHear: "A low hum"}, ""))
	require.NoError(t, editor.SaveChanged())
	routing := editor.Routing()
	routing.Ambient = map[string]AmbientProfile{"ambient/hum": {RadiusTiles: 8, FadeInMs: 800, FadeOutMs: 1200}}
	require.NoError(t, editor.SetRouting(routing))
	require.NoError(t, editor.SaveChanged())
	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, grownLibrary, string(saved))

	reopened, err := OpenLibraryEditor(path)
	require.NoError(t, err)
	require.Equal(t, int64(8), int64(reopened.Routing().Ambient["ambient/hum"].RadiusTiles))
	routing = reopened.Routing()
	routing.DefaultSurface = ""
	routing.Ambient["ambient/hum"] = AmbientProfile{RadiusTiles: 4, FadeInMs: 800, FadeOutMs: 1200}
	require.NoError(t, reopened.SetRouting(routing))
	require.NoError(t, reopened.SaveChanged())
	changed, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(changed), "\n      \"ambient/hum\": {\"radiusTiles\":4,")
	require.NotContains(t, string(changed), "null}", "sections the library lacks stay out of it")
}
