package ebitengine_test

import (
	"bytes"
	"errors"
	"image/color"
	"testing"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	backend "github.com/bradhannah/berad-sdk/modules/berad-graphics/backend/ebitengine"
	"github.com/hajimehoshi/ebiten/v2"
)

func processorWithLimits(t *testing.T, limits backend.Limits) *backend.Processor {
	t.Helper()
	processor, err := backend.NewProcessor(limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(processor.Close)
	return processor
}

func derive(t *testing.T, processor *backend.Processor, source *ebiten.Image, operations ...graphics.Operation) *ebiten.Image {
	t.Helper()
	result, err := processor.Derive(source, operations...)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProcessorSourceByteBudgetLRUAndSnapshotOwnership(t *testing.T) {
	processor := processorWithLimits(t, backend.Limits{MaxEntries: 10, SourceBytes: 8, DerivedBytes: 128})
	a, b, c := ebiten.NewImage(1, 1), ebiten.NewImage(1, 1), ebiten.NewImage(1, 1)
	for _, source := range []*ebiten.Image{a, b, c} {
		t.Cleanup(source.Deallocate)
		source.Fill(color.White)
	}
	oldA, oldB := derive(t, processor, a), derive(t, processor, b)
	if derive(t, processor, a) != oldA {
		t.Fatal("cache hit lost")
	}
	derive(t, processor, c) // Byte budget, not entry budget, evicts b's snapshot.
	stats := processor.Stats()
	if stats.Sources != 2 || stats.SourceBytes != 8 || stats.Derived != 3 || stats.DerivedBytes != 24 {
		t.Fatal("source budget/accounting", stats)
	}
	if derive(t, processor, a) != oldA {
		t.Fatal("recent source evicted")
	}
	b.Fill(color.RGBA{R: 64, A: 255}) // Evicted source can now be imported afresh.
	processor.Forget(b)
	freshB := derive(t, processor, b)
	if freshB == oldB || !bytes.Equal(pixels(freshB), []byte{64, 0, 0, 255}) || !bytes.Equal(pixels(oldB), []byte{255, 255, 255, 255}) {
		t.Fatal("source eviction did not separate snapshots or destroyed old result")
	}
	processor.Clear()
	if processor.Stats() != (backend.Stats{}) || pixels(oldA)[0] != 255 {
		t.Fatal("clear retained state or destroyed borrow")
	}
	if derive(t, processor, a) == oldA {
		t.Fatal("clear did not drop derivations")
	}
}

func TestProcessorDerivedByteBudgetLRUAndOversizedBypass(t *testing.T) {
	source := ebiten.NewImage(1, 1)
	t.Cleanup(source.Deallocate)
	source.Fill(color.White)
	processor := processorWithLimits(t, backend.Limits{MaxEntries: 10, SourceBytes: 4, DerivedBytes: 16})
	a, b, c := graphics.RGBGain(.2, graphics.Selection{}), graphics.RGBGain(.4, graphics.Selection{}), graphics.RGBGain(.6, graphics.Selection{})
	oldA, oldB := derive(t, processor, source, a), derive(t, processor, source, b)
	if derive(t, processor, source, a) != oldA {
		t.Fatal("cache hit lost")
	}
	derive(t, processor, source, c)
	if stats := processor.Stats(); stats.Derived != 2 || stats.DerivedBytes != 16 {
		t.Fatal("derived byte budget", stats)
	}
	if derive(t, processor, source, a) != oldA || derive(t, processor, source, b) == oldB {
		t.Fatal("derived LRU promotion")
	}
	if !bytes.Equal(pixels(oldB), []byte{102, 102, 102, 255}) {
		t.Fatal("evicted borrow changed")
	}
	oversized := processorWithLimits(t, backend.Limits{MaxEntries: 10, SourceBytes: 3, DerivedBytes: 7})
	first, second := derive(t, oversized, source), derive(t, oversized, source)
	if first == second || oversized.Stats() != (backend.Stats{}) || !bytes.Equal(pixels(first), []byte{255, 255, 255, 255}) {
		t.Fatal("oversized result must remain valid without retaining either cache")
	}
}

func TestProcessorAccountsRetainedMaskAndReplacement(t *testing.T) {
	source := ebiten.NewImage(2, 1)
	t.Cleanup(source.Deallocate)
	source.Fill(color.White)
	maskBytes := []byte{1, 0}
	mask, err := graphics.NewMask(2, 1, maskBytes)
	if err != nil {
		t.Fatal(err)
	}
	replacementBytes := []byte{20, 40, 60, 128, 0, 0, 0, 0}
	replacement, err := graphics.NewPixels(2, 1, replacementBytes)
	if err != nil {
		t.Fatal(err)
	}
	clear(maskBytes)
	clear(replacementBytes)
	operation := graphics.Replace(replacement, graphics.Masked(mask))
	for _, budget := range []int64{25, 26} {
		processor := processorWithLimits(t, backend.Limits{MaxEntries: 10, SourceBytes: 8, DerivedBytes: budget})
		first, second := derive(t, processor, source, operation), derive(t, processor, source, operation)
		retained := budget == 26 // 8 output + 8 source + 2 mask + 8 replacement.
		if (first == second) != retained {
			t.Fatal("operand admission boundary", budget)
		}
		wantBytes := int64(0)
		if retained {
			wantBytes = 26
		}
		if processor.Stats().DerivedBytes != wantBytes {
			t.Fatal("retained operand accounting", processor.Stats())
		}
		if !bytes.Equal(pixels(first), []byte{20, 40, 60, 128, 255, 255, 255, 255}) {
			t.Fatal("mask/replacement ownership or destination selection", pixels(first))
		}
		processor.Forget(source)
		if processor.Stats() != (backend.Stats{}) || pixels(first)[0] != 20 {
			t.Fatal("forget destroyed borrow or retained operands")
		}
	}
}

func TestProcessorLimitsAndTerminalErrors(t *testing.T) {
	for _, limits := range []backend.Limits{{}, {MaxEntries: -1, SourceBytes: 1, DerivedBytes: 1}, {MaxEntries: 1, SourceBytes: 0, DerivedBytes: 1}, {MaxEntries: 1, SourceBytes: 1, DerivedBytes: 0}} {
		if _, err := backend.NewProcessor(limits); !errors.Is(err, graphics.ErrInvalid) {
			t.Fatal("invalid budgets", limits, err)
		}
	}
	var zero backend.Processor
	processor := processorWithLimits(t, backend.DefaultLimits())
	processor.Close()
	for _, owner := range []*backend.Processor{nil, &zero, processor} {
		if _, err := owner.Derive(nil); !errors.Is(err, backend.ErrClosed) {
			t.Fatal("closed derive", err)
		}
		if _, err := owner.Silhouette(nil, 0); !errors.Is(err, backend.ErrClosed) {
			t.Fatal("closed silhouette", err)
		}
	}
}
