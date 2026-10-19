package catalog

import (
	"fmt"
	"html"
	"path"
	"slices"
	"strings"
)

// ReportOptions supplies display labels and reload guidance for generated output.
type ReportOptions struct{ ManifestLabel, AssetRootLabel, ReloadNotice string }

// GenerationGuide is a table-first projection; it never writes to the authored manifest.
func (catalog *Catalog) GenerationGuide(options ReportOptions) string {
	var out strings.Builder
	out.WriteString("# Audio library and generation guide\n\n")
	present := 0
	for _, asset := range catalog.Assets {
		if asset.Availability == CatalogPresent {
			present++
		}
	}
	fmt.Fprintf(&out, "**%d available recordings · %d empty recording slots · %d moment records**\n\n",
		present, len(catalog.Assets)-present, len(catalog.Moments))
	if options.ManifestLabel == "" {
		options.ManifestLabel = "the source audio manifest"
	}
	if options.AssetRootLabel == "" {
		options.AssetRootLabel = "the configured asset root"
	}
	out.WriteString("Edit **" + tableCell(options.ManifestLabel) + "**, not this generated report. ")
	out.WriteString("For an empty slot, set its recording file to an exact path relative to " +
		tableCell(options.AssetRootLabel) + ". " + tableCell(options.ReloadNotice) + " ")
	out.WriteString("To replace a shared recording for only one cue, add a new recording ID ")
	out.WriteString("and change that cue's `recordingId`.\n\n")
	out.WriteString("**How to hear it** describes the intended route; ")
	out.WriteString("every route still needs consumer integration and listening verification. ")
	out.WriteString("A code request or reference is evidence, not proof of audible playback. ")
	out.WriteString("All listening/calibration reviews are initially unreviewed.\n\n")
	catalog.writeCoverageSummary(&out)
	catalog.writeSFXTables(&out)
	writeMomentTable(&out, "Ambience", catalog.filteredMoments(func(moment CatalogMoment) bool {
		return moment.Category == "ambient" || moment.Category == "ambience"
	}))
	writeMomentTable(&out, "Music tracks", catalog.filteredMoments(func(moment CatalogMoment) bool {
		return moment.Category == "music" && moment.CueID != ""
	}))
	writeMomentTable(&out, "Music location and context bindings", catalog.filteredMoments(func(moment CatalogMoment) bool {
		return moment.Category == "music" && moment.CueID == ""
	}))
	writeMomentTable(&out, "Proposed moments", catalog.filteredMoments(func(moment CatalogMoment) bool {
		return strings.HasPrefix(moment.ID, "proposed/")
	}))
	catalog.writeRecordingTable(&out)
	catalog.writeProvenanceTable(&out)
	catalog.writeRecordingBriefs(&out)
	out.WriteString("## Evidence fingerprints\n\n")
	out.WriteString("- Supplied library: `" + catalog.LibrarySHA256 + "`\n")
	out.WriteString("- Scanned Go sources: `" + catalog.SourceTreeSHA256 + "`\n")
	out.WriteString("\nThe companion JSON contains full paths, SHA-256 hashes, source references, ")
	out.WriteString("gains, and independent review states.\n")

	return out.String()
}

// Static playback references are evidence of wiring, not reachability or
// listening approval; the count separates intentionally empty recording slots.
func (catalog *Catalog) writeCoverageSummary(out *strings.Builder) {
	type counts struct{ audible, silent, other int }
	groups := make(map[string]counts)
	for _, moment := range catalog.Moments {
		if moment.CueID == "" {
			continue
		}
		key := moment.Category + " / " + moment.Family
		group := groups[key]
		switch {
		case moment.TriggerCoverage != catalogPlaybackRequestFound:
			group.other++
		case moment.Availability == CatalogPresent:
			group.audible++
		default:
			group.silent++
		}
		groups[key] = group
	}
	out.WriteString("## Static coverage by category and intended family\n\n")
	out.WriteString("| Category / target family | Request + file | Request + silent slot | No request proven |\n")
	out.WriteString("|---|---:|---:|---:|\n")
	for _, key := range sortedKeys(groups) {
		group := groups[key]
		fmt.Fprintf(out, "| %s | %d | %d | %d |\n", tableCell(key), group.audible, group.silent, group.other)
	}
	out.WriteString("\nCounts use static code evidence, not verified gameplay routes or listening acceptance.\n\n")
}

func (catalog *Catalog) writeSFXTables(out *strings.Builder) {
	families := []string{}
	for _, moment := range catalog.Moments {
		if moment.Category == catalogSFX && !slices.Contains(families, moment.Family) {
			families = append(families, moment.Family)
		}
	}
	slices.Sort(families)
	for _, family := range families {
		writeMomentTable(out, "SFX — "+family, catalog.filteredMoments(func(moment CatalogMoment) bool {
			return moment.Category == catalogSFX && moment.Family == family
		}))
	}
}

func (catalog *Catalog) filteredMoments(keep func(CatalogMoment) bool) []CatalogMoment {
	moments := make([]CatalogMoment, 0)
	for _, moment := range catalog.Moments {
		if keep(moment) {
			moments = append(moments, moment)
		}
	}

	return moments
}

func writeMomentTable(out *strings.Builder, title string, moments []CatalogMoment) {
	out.WriteString("## " + tableCell(title) + "\n\n")
	out.WriteString("| Cue / moment | Intent / assignment | How to hear it in-game | Recording ID / file | ")
	out.WriteString("Asset need | Code evidence | Listening | Brief |\n")
	out.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, moment := range moments {
		recording := "—"
		if moment.AssetID != "" {
			recording = "<code>" + tableCell(strings.Join(moment.recordingIDs(), ", ")) + "</code>"
			if moment.Selection == SelectionCycle {
				recording += "<br>Cycle in order"
			}
			if moment.ConfiguredFile == "" {
				recording += "<br>file: null"
			} else {
				recording += "<br>" + tableCell(path.Base(moment.ConfiguredFile))
			}
		}
		how := moment.HowToHear
		if how == "" {
			how = "No established gameplay route; define the moment first."
		}
		if moment.Availability == CatalogMissing {
			how = "Not currently audible. " + how
		}
		brief := "—"
		if needsRecordingBrief(moment) {
			brief = "[Brief](#" + briefAnchor(moment.ID) + ")"
		}
		classification := tableCell(moment.Family)
		if moment.Phase != "" {
			classification += " · " + tableCell(moment.Phase) + "/" + tableCell(moment.Outcome) +
				" · " + tableCell(moment.MixRole)
		}
		if len(moment.AssignedFamilies) > 0 && !slices.Contains(moment.AssignedFamilies, moment.Family) {
			classification += "<br>Review assigned family: " + tableCell(strings.Join(moment.AssignedFamilies, ", "))
		}
		fmt.Fprintf(out, "| %s<br><code>%s</code> | %s | %s | %s | %s | %s | %s | %s |\n",
			tableCell(moment.Name), tableCell(moment.ID), classification, tableCell(how), recording,
			assetNeed(moment), evidenceLabel(moment.TriggerCoverage), tableCell(moment.ListeningReview), brief)
	}
	out.WriteString("\n")
}

func assetNeed(moment CatalogMoment) string {
	if strings.HasPrefix(moment.ID, "proposed/") || moment.TriggerCoverage == catalogDeclaredOnly ||
		moment.TriggerCoverage == catalogNoReference {
		return "Define/integrate use"
	}
	if moment.Availability == CatalogMissing {
		return "Supply recording"
	}
	if moment.AssignmentReview == CatalogIncorrect {
		return "Replace"
	}
	if len(moment.AssignedFamilies) > 0 && !slices.Contains(moment.AssignedFamilies, moment.Family) {
		return "Review family mismatch"
	}

	return "Review existing"
}

func evidenceLabel(coverage string) string {
	switch coverage {
	case catalogPlaybackRequestFound:
		return "Playback request found"
	case "source_reference_found":
		return "Reference found; trace route"
	case catalogNoReference:
		return "Declared; no caller found"
	case catalogDeclaredOnly, "proposed_not_integrated":
		return "Not integrated"
	default:
		return "Needs code check"
	}
}

func (catalog *Catalog) writeRecordingTable(out *strings.Builder) {
	out.WriteString("## Recordings and prepared slots\n\n")
	out.WriteString("| Recording ID | Acoustic family | Exact file (relative to assets) | State | " +
		"Source duration (ms) | " +
		"Clip region (ms) | Moment uses | Source |\n")
	out.WriteString("|---|---|---|---|---:|---|---:|---|\n")
	for _, asset := range catalog.Assets {
		file := strings.TrimPrefix(asset.Path, "assets/")
		if file == "" {
			file = "null — awaiting recording"
		}
		source := asset.Provenance.SourceURL
		state := asset.Availability
		if !asset.Registered {
			state += "; unregistered file"
		}
		region := "full source"
		if asset.Window != nil {
			region = fmt.Sprintf("%d–%d", asset.Window.StartMs, asset.Window.EndMs)
		}
		fmt.Fprintf(out, "| <code>%s</code> | %s | %s | %s | %d | %s | %d | %s |\n",
			tableCell(asset.ID), tableCell(asset.AcousticFamily), tableCell(file), tableCell(state), asset.DurationMs,
			region, len(asset.MomentIDs), tableCell(source))
	}
	out.WriteString("\n")
}

func (catalog *Catalog) writeProvenanceTable(out *strings.Builder) {
	out.WriteString("## Recording provenance\n\n")
	out.WriteString("Generated from the manifest. Unknown source details stay **unknown**; ")
	out.WriteString("conversion/editing credit does not establish source authorship or licensing. ")
	out.WriteString("Empty recording slots also carry explicit unknown provenance in JSON.\n\n")
	out.WriteString("| Recording ID | Original filename | Source type | Source / creator | ")
	out.WriteString("License | Processing | Import lineage |\n")
	out.WriteString("|---|---|---|---|---|---|---|\n")
	for _, asset := range catalog.Assets {
		if asset.Availability != CatalogPresent {
			continue
		}
		provenance := asset.Provenance
		processing := []string{}
		for _, step := range provenance.Processing {
			processing = append(processing, step.Description+" Tool: "+step.Tool+"; by: "+step.PerformedBy+".")
		}
		if len(processing) == 0 {
			processing = append(processing, provenanceUnknown)
		}
		fmt.Fprintf(out, "| <code>%s</code> | %s | %s | %s<br>%s | %s | %s | %s |\n",
			tableCell(asset.ID), tableCell(provenance.OriginalFilename), tableCell(provenance.SourceType),
			tableCell(provenance.SourceURL), tableCell(provenance.SourceCreator), tableCell(provenance.License),
			tableCell(strings.Join(processing, "\n")), tableCell(provenance.ImportedFrom))
	}
	out.WriteString("\n")
}

func needsRecordingBrief(moment CatalogMoment) bool {
	if moment.GenerationBrief == "" || moment.TriggerCoverage == catalogDeclaredOnly ||
		moment.TriggerCoverage == catalogNoReference {
		return false
	}

	return moment.Availability == CatalogMissing || moment.AssignmentReview == CatalogIncorrect
}

func (catalog *Catalog) writeRecordingBriefs(out *strings.Builder) {
	out.WriteString("## Recording briefs\n\n")
	out.WriteString("Drafts for recording gaps or incorrect assignments; not automatic generation requirements.\n\n")
	for _, moment := range catalog.Moments {
		if !needsRecordingBrief(moment) {
			continue
		}
		out.WriteString("<a id=\"" + briefAnchor(moment.ID) + "\"></a>\n\n")
		out.WriteString("### " + tableCell(moment.Name) + "\n\n")
		out.WriteString("Cue: <code>" + tableCell(moment.ID) + "</code> · Recording slots: <code>" +
			tableCell(strings.Join(moment.recordingIDs(), ", ")) + "</code>\n\n")
		out.WriteString(tableCell(moment.GenerationBrief) + "\n\n")
		if moment.Note != "" {
			out.WriteString(tableCell(moment.Note) + "\n\n")
		}
	}
}

func briefAnchor(identity string) string {
	return "brief-" + strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			return character
		}

		return '-'
	}, strings.ToLower(identity))
}

func tableCell(text string) string {
	return strings.NewReplacer("|", "\\|", "\r", "", "\n", "<br>").Replace(html.EscapeString(text))
}
