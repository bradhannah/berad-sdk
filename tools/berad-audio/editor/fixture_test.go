package editorui

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	audio "github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
)

// Generated project data preserves useful historic regression identities without
// loading a game manifest, game packages, original data, or production recordings.
func testDocument() audio.LibraryDocument {
	document := audio.LibraryDocument{SchemaVersion: 3, Recordings: map[string]audio.Recording{}, Cues: map[string]audio.Cue{},
		Taxonomy: audio.Taxonomy{RecordingFamilies: map[string][]string{}, CueProfiles: map[string][]string{}, Profiles: map[string]audio.CueClassification{}},
		Routing:  audio.Routing{Ambient: map[string]audio.AmbientProfile{}, Tiles: map[string]audio.TileAudio{}, Surfaces: map[string][]string{}, Emitters: map[string]string{}, Groups: map[string]map[string]*string{}}}
	families := map[string][]string{"magic": {"healing_01", "healing_02", "field_dispel_01"}, "mechanical_interaction": {"push_pull_01", "physical_unlock_01", "door_open_01"},
		"footstep": {"stone_steps_01", "grass_steps_01"}, "fire": {"cardboard_burning_01"}, "ambience": {"forest_nature_01", "fountain_01", "stream_01", "boat_01"}, "score": {"ultima_theme", "town_01", "hornpipe_01"}}
	for index := 0; index < 60; index++ {
		families["unreviewed"] = append(families["unreviewed"], fmt.Sprintf("unused_%03d", index))
	}
	for family, recordings := range families {
		document.Taxonomy.RecordingFamilies[family] = recordings
		for _, id := range recordings {
			document.Recordings[id] = audio.Recording{Provenance: audio.RecordingProvenance{OriginalFilename: "unknown", SourceType: "generated", SourceURL: "unknown", SourceCreator: "test generator", License: "MIT", ImportedFrom: "generated fixture", Processing: []audio.ProcessingStep{}}}
		}
	}
	profiles := map[string]string{"magic_activation": "magic", "interaction_action": "mechanical_interaction", "environment_harm": "impact", "footstep": "footstep", "ambient_bed": "ambience", "score": "score", "fire": "fire"}
	for name, family := range profiles {
		document.Taxonomy.Profiles[name] = audio.CueClassification{TargetAcousticFamily: family, Phase: "action", Outcome: "success", MixRole: "foreground_feedback", Emitters: []string{"panel"}, Contexts: []string{"preview"}}
	}
	rows := [][4]string{
		{"sfx/SoundHeal", "healing_01", "magic_activation", "Restore an injured character"},
		{"sfx/SoundDispel", "field_dispel_01", "magic_activation", "Dispel"},
		{"sfx/SoundHealOther", "healing_02", "magic_activation", "Other healing"},
		{"sfx/SoundPushObject", "push_pull_01", "interaction_action", "Push object"},
		{"sfx/SoundUnlock", "physical_unlock_01", "interaction_action", "Use a lockpick"},
		{"sfx/SoundWalkMetal1", "stone_steps_01", "footstep", "Walk on grating"},
		{"sfx/SoundWalkMetal2", "stone_steps_01", "footstep", "Stone step"},
		{"sfx/SoundWalkGrassLeft", "grass_steps_01", "footstep", "Walk on grass"},
		{"sfx/SoundWalkGrassRight", "grass_steps_01", "footstep", "Walk on grass right"},
		{"sfx/SoundFireCrackle", "cardboard_burning_01", "fire", "Fire crackle"},
		{"music/track/town", "town_01", "score", "Monarch"}, {"music/track/overworld", "ultima_theme", "score", "Exploration"},
		{"music/track/hornpipe", "hornpipe_01", "score", "Sailing music"},
		{"ambient/AmbientFire", "cardboard_burning_01", "ambient_bed", "Fire"},
		{"ambient/AmbientFountain", "fountain_01", "ambient_bed", "Fountain"},
		{"ambient/AmbientForest", "forest_nature_01", "ambient_bed", "Forest"},
		{"ambient/AmbientStream", "stream_01", "ambient_bed", "Stream"},
		{"ambient/frigate_onboard", "boat_01", "ambient_bed", "Frigate"}, {"ambient/skiff_onboard", "boat_01", "ambient_bed", "Skiff"},
	}
	for _, row := range rows {
		document.Cues[row[0]] = audio.Cue{Name: row[3], RecordingID: row[1], VolumeModifier: 1, HowToHear: row[3]}
		document.Taxonomy.CueProfiles[row[2]] = append(document.Taxonomy.CueProfiles[row[2]], row[0])
	}
	for _, id := range []string{"ambient/AmbientFire", "ambient/AmbientFountain", "ambient/AmbientForest", "ambient/AmbientStream"} {
		document.Routing.Ambient[id] = audio.AmbientProfile{RadiusTiles: 8, FadeInMs: 800, FadeOutMs: 1000}
	}
	for _, id := range []string{"ambient/frigate_onboard", "ambient/skiff_onboard"} {
		document.Routing.Ambient[id] = audio.AmbientProfile{}
	}
	document.Routing.Tiles["216"] = audio.TileAudio{AmbientCue: "ambient/AmbientFountain"}
	document.Routing.Tiles["178"] = audio.TileAudio{AmbientCue: "ambient/AmbientFire"}
	document.Routing.Tiles["269"] = audio.TileAudio{AmbientCue: "ambient/AmbientFire"}
	document.Routing.Tiles["68"] = audio.TileAudio{Surface: "stone"}
	document.Routing.Surfaces["stone"] = []string{"sfx/SoundWalkMetal1", "sfx/SoundWalkMetal2"}
	document.Routing.Surfaces["grass"] = []string{"sfx/SoundWalkGrassLeft", "sfx/SoundWalkGrassRight"}
	document.Routing.Surfaces["metal"] = []string{"sfx/SoundWalkMetal1"}
	document.Routing.DefaultSurface = "grass"
	ptr := func(value string) *string { return &value }
	document.Routing.Groups["music"] = map[string]*string{"default": ptr("music/track/overworld")}
	document.Routing.Groups["dungeon"] = map[string]*string{}
	document.Routing.Groups["naval"] = map[string]*string{"frigate": ptr("ambient/frigate_onboard"), "skiff": ptr("ambient/skiff_onboard"), "sailing": ptr("music/track/hornpipe")}
	return document
}

func testWAV() []byte {
	data := calibrationWAV(4000)
	data = append(data[:44], bytes.Repeat(data[44:], 6)...)
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	return data
}

func testSliderGain(value int) float64 {
	return [...]float64{0, .01, .03, .07, .18, .30, .43, .55, .65, .72, .75}[value]
}

func testAuthoringOptions() audio.EditorOptions {
	options := audio.DefaultEditorOptions(ebitengine.Decode)
	options.Calibration = func(family string) audio.CalibrationOptions {
		value := audio.DefaultCalibrationOptions()
		switch family {
		case "score", "instrument":
			value.Method, value.TargetDB = audio.GatedLoudness, -23
		case "ambience", "nature", "fire":
			value.Method, value.TargetDB = audio.GatedLoudness, -26
		}
		return value
	}
	options.Targets = audio.AuthoringTargets{Tiles: []audio.TileTarget{{ID: "216", Name: "Fountain", Frames: []int{216, 217, 218, 219}}, {ID: "178", Name: "Fire", Frames: []int{178}}, {ID: "269", Name: "Other fire", Frames: []int{269}}, {ID: "68", Name: "Grass", Frames: []int{68}}},
		Groups: map[string]audio.AssignmentGroup{
			"music":   {Label: "music", Category: "music", DefaultTarget: "default", Targets: map[string]string{"default": "Default music", "location/2": "Test town"}},
			"dungeon": {Label: "background", Category: "ambient", DefaultTarget: "default", Targets: map[string]string{"default": "Default background", "location/33": "Test cave"}},
			"naval":   {Label: "situation", Category: "ambient", Targets: map[string]string{"frigate": "Frigate", "skiff": "Skiff", "sailing": "Sailing"}}},
		Families: []string{"ambience", "fire", "footstep", "impact", "magic", "mechanical_interaction", "nature", "score", "status", "unreviewed"}, CueCategories: map[string]string{}}
	for id := range testDocument().Cues {
		category := "sfx"
		if strings.HasPrefix(id, "music/") {
			category = "music"
		}
		if strings.HasPrefix(id, "ambient/") {
			category = "ambient"
		}
		options.Targets.CueCategories[id] = category
	}
	return options
}

func openTestEditor(path string) (*audio.LibraryEditor, error) {
	return audio.OpenLibraryProject(path, testAuthoringOptions())
}

func testViewOptions() Options {
	return Options{SliderGain: testSliderGain, MixLabel: "GAME MIX",
		RecordingCategory: func(_ string, _ audio.Recording, family string) string {
			if family == "score" {
				return "music"
			}
			if family == "ambience" {
				return "ambient"
			}
			return "sfx"
		},
		TargetPolicy: func(group, target string) TargetPolicy {
			if group == "music" || group == "naval" && target == "sailing" {
				title := "CHOOSE MUSIC"
				if group == "naval" {
					title = "CHOOSE SAILING MUSIC"
				}
				return TargetPolicy{Category: "music", Title: title}
			}
			if group == "naval" {
				return TargetPolicy{Category: "ambient", Background: true, Title: "CHOOSE ONBOARD AMBIENCE"}
			}
			if group == "dungeon" {
				return TargetPolicy{Category: "ambient", Background: true, Title: "CHOOSE DUNGEON BACKGROUND"}
			}
			return TargetPolicy{Category: "ambient", Spatial: true}
		},
		Ambient: &AmbientPreset{CuePrefix: "ambient/", Profile: "ambient_bed", Family: "ambience", Gain: .3, HowToHear: "Assign this sound to a preview source"}}
}
