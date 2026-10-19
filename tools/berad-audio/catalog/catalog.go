package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
)

// PlaybackWindow preserves the shared authoring window schema in catalog views.
type PlaybackWindow = authoring.PlaybackWindow

// RecordingProvenance preserves source attribution in generated reports.
type RecordingProvenance = authoring.RecordingProvenance

// SelectionCycle identifies ordered recording variants.
const SelectionCycle = authoring.SelectionCycle
const catalogPlaybackRequestFound = "playback_request_found"
const provenanceUnknown = "unknown"

// Options makes decoding, measurements and category policy explicit. PathPrefix
// is a report label only; Assets passed to Build is already rooted by the caller.
type Options struct {
	Decode        audio.Decoder
	Measure       bool
	PathPrefix    string
	CueCategories map[string]string
}

// Catalog schema and independent recording/review states.
const (
	CatalogSchemaVersion = 2
	CatalogUnreviewed    = "unreviewed"
	CatalogPresent       = "present"
	CatalogMissing       = "missing"
	CatalogIncorrect     = "incorrect"
	CatalogPlaceholder   = "placeholder"
	catalogNotScanned    = "not_scanned"
	catalogDeclaredOnly  = "declared_only"
	catalogNoReference   = "no_static_reference_found"
	catalogSFX           = "sfx"
	catalogMusic         = "music"
	catalogAmbience      = "ambience"
)

// ErrAudioCatalog indicates inconsistent catalog identity or inventory data.
var ErrAudioCatalog = errors.New("invalid audio catalog")

// Catalog is a detached, read-only inventory, not a runtime playback config or
// tuning handoff. Assignments and coefficients are derived from production code.
type Catalog struct {
	SchemaVersion    int             `json:"schemaVersion"`
	LibrarySHA256    string          `json:"librarySha256"`
	SourceTreeSHA256 string          `json:"sourceTreeSha256,omitempty"`
	Assets           []CatalogAsset  `json:"assets"`
	Moments          []CatalogMoment `json:"moments"`
}

// CatalogAsset describes one exact embedded recording. Null calibration means
// no accepted calibration exists; it does not invent a 0dB approval.
//
//nolint:revive // Preserve the published CatalogAsset API name for existing consumers.
type CatalogAsset struct {
	ID                  string              `json:"id"`
	AcousticFamily      string              `json:"acousticFamily,omitempty"`
	SearchTerms         []string            `json:"searchTerms,omitempty"`
	Registered          bool                `json:"registered"`
	Availability        string              `json:"availability"`
	Path                string              `json:"path"`
	SHA256              string              `json:"sha256"`
	Format              string              `json:"format"`
	DurationMs          int64               `json:"durationMs"`
	PCMMeasured         bool                `json:"pcmMeasured"`
	PeakDBFS            *float64            `json:"samplePeakDbfs"`
	RMSDBFS             *float64            `json:"rmsDbfs"`
	CalibrationGainDB   *float64            `json:"calibrationGainDb"`
	CalibrationMode     string              `json:"calibrationMode"`
	CalibrationTargetDB *float64            `json:"calibrationTargetDb,omitempty"`
	Window              *PlaybackWindow     `json:"playbackWindow,omitempty"`
	ListeningReview     string              `json:"listeningReview"`
	LoopReview          string              `json:"loopReview"`
	Provenance          RecordingProvenance `json:"provenance"`
	Note                string              `json:"note"`
	MomentIDs           []string            `json:"momentIds"`
}

// CatalogMoment describes one semantic cue or musical/environmental context.
// Availability, assignment suitability, trigger evidence, and listening review
// are independent. A source reference is not a completed gameplay walkthrough.
//
//nolint:revive // Preserve the published CatalogMoment API name for existing consumers.
type CatalogMoment struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Category         string            `json:"category"`
	Family           string            `json:"family"`
	AssignedFamilies []string          `json:"assignedAcousticFamilies,omitempty"`
	MixRole          string            `json:"mixRole"`
	Phase            string            `json:"phase,omitempty"`
	Outcome          string            `json:"outcome,omitempty"`
	Emitters         []string          `json:"emitters,omitempty"`
	Contexts         []string          `json:"contexts,omitempty"`
	CueID            string            `json:"cueId,omitempty"`
	HowToHear        string            `json:"howToHear"`
	SearchTerms      []string          `json:"searchTerms,omitempty"`
	AssetID          string            `json:"assetId,omitempty"`
	AssetIDs         []string          `json:"assetIds,omitempty"`
	Selection        string            `json:"selection,omitempty"`
	ConfiguredFile   string            `json:"configuredFile,omitempty"`
	Availability     string            `json:"availability"`
	AssignmentReview string            `json:"assignmentReview"`
	ListeningReview  string            `json:"listeningReview"`
	TriggerCoverage  string            `json:"triggerCoverage"`
	SourceSymbols    []string          `json:"sourceSymbols"`
	Evidence         []CatalogEvidence `json:"evidence"`
	Playback         string            `json:"playback"`
	Overlap          string            `json:"overlap"`
	ConfiguredGain   *float64          `json:"configuredGain"`
	GainSemantics    string            `json:"gainSemantics"`
	Window           *PlaybackWindow   `json:"playbackWindow"`
	MaxDistanceTiles *float64          `json:"maxDistanceTiles"`
	FadeInMs         int64             `json:"fadeInMs"`
	FadeOutMs        int64             `json:"fadeOutMs"`
	SpriteIndices    []int             `json:"spriteIndices"`
	GenerationBrief  string            `json:"generationBrief"`
	Note             string            `json:"note"`
	RelatedIssues    []int             `json:"relatedIssues"`
}

// CatalogEvidence points to a static source reference, never inferred acceptance.
//
//nolint:revive // Preserve the published CatalogEvidence API name for existing consumers.
type CatalogEvidence struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Function string `json:"function"`
	Symbol   string `json:"symbol"`
	Kind     string `json:"kind"`
}

// Build never creates players or an audio context. It hashes the exact supplied
// manifest and reads only the explicitly supplied asset root. Measurements are
// optional; source-code evidence is supplied separately by a consumer.
func Build(manifest []byte, files fs.FS, options Options) (*Catalog, error) {
	if files == nil || options.Decode == nil {
		return nil, fmt.Errorf("%w: asset root and decoder required", ErrAudioCatalog)
	}
	library, err := authoring.ParseDocument(manifest, nil)
	if err != nil {
		return nil, fmt.Errorf("parse catalog manifest: %w", err)
	}
	hash := sha256.Sum256(manifest)
	catalog := &Catalog{
		SchemaVersion: CatalogSchemaVersion, LibrarySHA256: hex.EncodeToString(hash[:]),
		SourceTreeSHA256: "", Assets: nil, Moments: nil,
	}
	if err := catalog.addLibraryRecordings(library, files, options); err != nil {
		return nil, err
	}
	for _, id := range sortedKeys(library.Cues) {
		cue := library.Cues[id]
		moment := newCatalogMoment(id, cue.Name, options.CueCategories[id])
		catalog.assignCue(&moment, id, library, options)
		catalog.Moments = append(catalog.Moments, moment)
	}
	catalog.linkAssets()
	slices.SortFunc(catalog.Assets, func(a, b CatalogAsset) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(catalog.Moments, func(a, b CatalogMoment) int { return strings.Compare(a.ID, b.ID) })
	if err := catalog.Validate(); err != nil {
		return nil, err
	}

	return catalog, nil
}

func newCatalogMoment(id, name, category string) CatalogMoment {
	var moment CatalogMoment
	moment.ID, moment.Name, moment.Category = id, name, category
	moment.Availability = "missing_definition"
	moment.AssignmentReview, moment.ListeningReview = CatalogUnreviewed, CatalogUnreviewed
	moment.TriggerCoverage = catalogNotScanned
	moment.Evidence = []CatalogEvidence{}
	moment.SourceSymbols = []string{}
	moment.SpriteIndices = []int{}
	moment.RelatedIssues = []int{}
	moment.Playback = "not configured"
	moment.Overlap = "not configured"

	return moment
}

func (catalog *Catalog) assignCue(
	moment *CatalogMoment, cueID string, library authoring.LibraryDocument, options Options,
) {
	entry := library.Cues[cueID]
	profile := groupFor(library.Taxonomy.CueProfiles, cueID)
	moment.SearchTerms = append(slices.Clone(entry.SearchTerms),
		library.Taxonomy.ProfileSearchTerms[profile]...)
	classification := library.Taxonomy.Profiles[profile]
	moment.Category = options.CueCategories[cueID]
	if moment.Category == "" {
		moment.Category = catalogSFX
		if _, ambient := library.Routing.Ambient[cueID]; ambient {
			moment.Category = "ambient"
		}
	}
	moment.Family, moment.Phase = classification.TargetAcousticFamily, classification.Phase
	moment.Outcome = classification.Outcome
	moment.MixRole = classification.MixRole
	moment.Emitters, moment.Contexts = slices.Clone(classification.Emitters), slices.Clone(classification.Contexts)
	moment.CueID, moment.AssetID = cueID, entry.PrimaryRecording()
	for _, recordingID := range entry.Recordings() {
		family := groupFor(library.Taxonomy.RecordingFamilies, recordingID)
		if !slices.Contains(moment.AssignedFamilies, family) {
			moment.AssignedFamilies = append(moment.AssignedFamilies, family)
		}
	}
	if entry.Selection == SelectionCycle {
		moment.AssetIDs = entry.Recordings()
		moment.Selection = entry.Selection
	}
	moment.HowToHear, moment.ConfiguredGain = entry.HowToHear, &entry.VolumeModifier
	if file := library.Recordings[entry.PrimaryRecording()].File; file != nil {
		moment.ConfiguredFile = options.PathPrefix + *file
	}
	if window := library.Recordings[entry.PrimaryRecording()].PlaybackWindow; window != nil {
		selection := *window
		moment.Window = &selection
	}
	moment.Availability = CatalogPresent
	if moment.ConfiguredFile == "" {
		moment.Availability, moment.AssignmentReview = CatalogMissing, CatalogPlaceholder
	}
}

func (catalog *Catalog) addLibraryRecordings(library authoring.LibraryDocument, files fs.FS, options Options) error {
	registeredPaths := make(map[string]bool)
	for recordingID, recording := range library.Recordings {
		path := ""
		if recording.File != nil {
			path = *recording.File
		}
		var asset CatalogAsset
		if path == "" {
			asset.ID, asset.Registered, asset.Availability = recordingID, true, CatalogMissing
			asset.ListeningReview, asset.LoopReview = CatalogUnreviewed, CatalogUnreviewed
		} else {
			var err error
			asset, err = InspectAsset(recordingID, path, files, options)
			if err != nil {
				return err
			}
			registeredPaths[path] = true
			asset.Registered = true
		}
		asset.Provenance = recording.Provenance
		asset.AcousticFamily = groupFor(library.Taxonomy.RecordingFamilies, recordingID)
		asset.SearchTerms = slices.Clone(recording.SearchTerms)
		asset.CalibrationMode = recording.CalibrationMode()
		if recording.CalibrationTargetDB != nil {
			target := *recording.CalibrationTargetDB
			asset.CalibrationTargetDB = &target
		}
		if recording.PlaybackWindow != nil {
			window := *recording.PlaybackWindow
			asset.Window = &window
		}
		asset.Provenance.Processing = slices.Clone(recording.Provenance.Processing)
		asset.Note = recording.Note
		if recording.CalibrationGainDB != nil {
			gain := *recording.CalibrationGainDB
			asset.CalibrationGainDB = &gain
		}
		catalog.Assets = append(catalog.Assets, asset)
	}
	err := fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk embedded recordings: %w", err)
		}
		if entry.IsDir() || registeredPaths[path] || formatName(path) == provenanceUnknown {
			return nil
		}
		asset, err := InspectAsset("unregistered/"+path, path, files, options)
		if err != nil {
			return err
		}
		catalog.Assets = append(catalog.Assets, asset)

		return nil
	})
	if err != nil {
		return fmt.Errorf("inventory unregistered audio: %w", err)
	}

	return nil
}

func (catalog *Catalog) linkAssets() {
	for i := range catalog.Assets {
		asset := &catalog.Assets[i]
		asset.MomentIDs = []string{}
		for _, moment := range catalog.Moments {
			if slices.Contains(moment.recordingIDs(), asset.ID) {
				asset.MomentIDs = append(asset.MomentIDs, moment.ID)
			}
		}
		slices.Sort(asset.MomentIDs)
	}
}

// Validate checks graph identity and assignment integrity. Catalog annotations
// must not conceal missing assets or orphan an inverse usage link.
func (catalog *Catalog) Validate() error {
	if catalog.SchemaVersion != CatalogSchemaVersion {
		return fmt.Errorf("%w: unsupported schema %d", ErrAudioCatalog, catalog.SchemaVersion)
	}
	assets := make(map[string]CatalogAsset, len(catalog.Assets))
	for _, asset := range catalog.Assets {
		if _, duplicate := assets[asset.ID]; duplicate || asset.ID == "" {
			return fmt.Errorf("%w: duplicate/empty asset identity %q", ErrAudioCatalog, asset.ID)
		}
		assets[asset.ID] = asset
		if asset.Registered && asset.AcousticFamily == "" {
			return fmt.Errorf("%w: recording %s has no acoustic family", ErrAudioCatalog, asset.ID)
		}
	}
	seen := make(map[string]bool, len(catalog.Moments))
	for _, moment := range catalog.Moments {
		if seen[moment.ID] || moment.ID == "" {
			return fmt.Errorf("%w: duplicate/empty moment identity %q", ErrAudioCatalog, moment.ID)
		}
		seen[moment.ID] = true
		if err := validateCatalogMoment(moment, assets); err != nil {
			return err
		}
	}

	return catalog.validateInverseLinks()
}

func validateCatalogMoment(moment CatalogMoment, assets map[string]CatalogAsset) error {
	if moment.CueID != "" && (moment.Family == "" || moment.Phase == "" || moment.Outcome == "" || moment.MixRole == "") {
		return fmt.Errorf("%w: unclassified moment %s", ErrAudioCatalog, moment.ID)
	}
	if len(moment.AssetIDs) > 0 && (moment.Selection != SelectionCycle ||
		len(moment.AssetIDs) < 2 || moment.AssetID != moment.AssetIDs[0]) {
		return fmt.Errorf("%w: invalid ordered assignment for %s", ErrAudioCatalog, moment.ID)
	}
	if moment.AssetID == "" {
		if moment.Availability == CatalogPresent {
			return fmt.Errorf("%w: present moment %s has no asset", ErrAudioCatalog, moment.ID)
		}

		return nil
	}

	return validateMomentAssets(moment, assets)
}

func validateMomentAssets(moment CatalogMoment, assets map[string]CatalogAsset) error {
	for _, recordingID := range moment.recordingIDs() {
		asset, exists := assets[recordingID]
		if !exists || !slices.Contains(asset.MomentIDs, moment.ID) || moment.Availability != asset.Availability {
			return fmt.Errorf("%w: unresolved assignment for %s", ErrAudioCatalog, moment.ID)
		}
	}

	return nil
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

func groupFor(groups map[string][]string, id string) string {
	for group, ids := range groups {
		if slices.Contains(ids, id) {
			return group
		}
	}

	return ""
}

// Relink refreshes inverse relationships after consumer-specific moments are added.
func (catalog *Catalog) Relink() { catalog.linkAssets() }

func (catalog *Catalog) validateInverseLinks() error {
	for _, asset := range catalog.Assets {
		for _, momentID := range asset.MomentIDs {
			matches := slices.ContainsFunc(catalog.Moments, func(moment CatalogMoment) bool {
				return moment.ID == momentID && slices.Contains(moment.recordingIDs(), asset.ID)
			})
			if !matches {
				return fmt.Errorf("%w: stale usage %s on %s", ErrAudioCatalog, momentID, asset.ID)
			}
		}
	}

	return nil
}

func (moment CatalogMoment) recordingIDs() []string {
	if len(moment.AssetIDs) > 0 {
		return moment.AssetIDs
	}
	if moment.AssetID != "" {
		return []string{moment.AssetID}
	}

	return nil
}
