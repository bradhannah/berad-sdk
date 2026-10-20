package catalog_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/catalog"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/internal/testproject"
)

func TestExplicitManifestMeasurementAndDetachedReport(t *testing.T) {
	_, root, data := testproject.Write(t)
	value, err := catalog.Build(data, os.DirFS(root), catalog.Options{Decode: ebitengine.Decode, Measure: true, CueCategories: map[string]string{"panel/accept": "music"}})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	if value.LibrarySHA256 != hex.EncodeToString(hash[:]) || len(value.Assets) != 1 || len(value.Moments) != 1 {
		t.Fatal("wrong inventory or provenance")
	}
	asset := value.Assets[0]
	if asset.DurationMs != 10 || !asset.PCMMeasured || asset.RMSDBFS == nil || math.Abs(*asset.RMSDBFS-20*math.Log10(.125)) > 1e-8 {
		t.Fatalf("incorrect measurement: %+v", asset)
	}
	if value.Moments[0].Category != "music" || len(asset.MomentIDs) != 1 {
		t.Fatal("category or inverse link lost")
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.Moments[0].Family = "<script>|x\r\nnext"
	report := value.GenerationGuide(catalog.ReportOptions{ManifestLabel: "chosen.json"})
	if strings.Contains(report, "<script>") || !strings.Contains(report, "&lt;script&gt;\\|x<br>next") || !strings.Contains(report, "chosen.json") {
		t.Fatal("authored report values not escaped")
	}
	if strings.Contains(report, "internal/audio/assets") || value.SourceTreeSHA256 != "" {
		t.Fatal("implicit game source/evidence")
	}
}

func TestIntentionalEmptySlotDoesNotDecode(t *testing.T) {
	_, root, data := testproject.Write(t)
	doc, err := authoring.ParseDocument(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	recording := doc.Recordings["signal"]
	recording.File = nil
	doc.Recordings["signal"] = recording
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	// An explicit empty root means no unregistered recording inventory either.
	empty := t.TempDir()
	value, err := catalog.Build(data, os.DirFS(empty), catalog.Options{Decode: ebitengine.Decode})
	if err != nil {
		t.Fatal(root, err)
	}
	if value.Assets[0].Availability != catalog.CatalogMissing || value.Moments[0].Availability != catalog.CatalogMissing {
		t.Fatal("empty slot lost")
	}
}
