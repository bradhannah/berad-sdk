// Package ebitengine adapts supplied graphics resources to Ebitengine. Callers
// serialize processor access on their render lane and retain ownership of targets.
package ebitengine

import (
	"container/list"
	"errors"
	"fmt"
	"image"

	graphics "github.com/bradhannah/berad-sdk/modules/berad-graphics"
	"github.com/hajimehoshi/ebiten/v2"
)

const maximumOperations = 16

// ErrClosed rejects new work after processor/surface closure or on an uninitialized processor.
var ErrClosed = errors.New("graphics resource closed")

// Limits bound source snapshots and derived GPU references separately. Borrowed
// results retained by hosts are outside cache accounting; this is not total VRAM.
type Limits struct {
	MaxEntries                int
	SourceBytes, DerivedBytes int64
}

// DefaultLimits supplies bounded source/derived stores, each with 512 entries and 32 MiB.
func DefaultLimits() Limits {
	const entries = 512
	const payload = 32 << 20

	return Limits{MaxEntries: entries, SourceBytes: payload, DerivedBytes: payload}
}

type sourceEntry struct {
	source   *ebiten.Image
	snapshot *graphics.Image
	element  *list.Element
}

type derivedKey struct {
	source     *graphics.Image
	operations [maximumOperations]graphics.Operation
	count      int
	silhouette bool
	threshold  byte
}

type derivedEntry struct {
	key     derivedKey
	image   *ebiten.Image
	bytes   int64
	element *list.Element
}

// Stats reports cache-held payload, never servicing rendering.
type Stats struct {
	Sources, Derived          int
	SourceBytes, DerivedBytes int64
}

// Processor snapshots explicitly borrowed immutable GPU sources. Call Forget
// after host mutation before asking for a new snapshot. No automatic reload occurs.
// Returned derived images are read-only borrows: eviction/Close drops references
// without deallocating images a host still holds. The host must not dispose them.
type Processor struct {
	limits                Limits
	sources               map[*ebiten.Image]*sourceEntry
	derived               map[derivedKey]*derivedEntry
	sourceLRU, derivedLRU list.List
	stats                 Stats
	closed                bool
}

// NewProcessor validates explicit positive cache budgets.
func NewProcessor(limits Limits) (*Processor, error) {
	if limits.MaxEntries <= 0 || limits.SourceBytes <= 0 || limits.DerivedBytes <= 0 {
		return nil, fmt.Errorf("%w: positive graphics cache limits required", graphics.ErrInvalid)
	}
	processor := new(Processor)
	processor.limits = limits
	processor.sources = make(map[*ebiten.Image]*sourceEntry)
	processor.derived = make(map[derivedKey]*derivedEntry)

	return processor, nil
}

func (processor *Processor) ready() bool {
	return processor != nil && !processor.closed && processor.sources != nil
}

// imageBounds handles only Ebitengine's known disposed-image panic; other programming
// or initialization failures are not converted to successful fallback artwork.
//
//nolint:nonamedreturns // Deferred disposed-image recovery must return an explicit error.
func imageBounds(source *ebiten.Image) (bounds image.Rectangle, err error) {
	if source == nil {
		return bounds, fmt.Errorf("%w: nil image", graphics.ErrInvalid)
	}
	defer func() {
		if failure := recover(); failure != nil {
			if message, ok := failure.(string); !ok || message != "ebiten: the image is already disposed" {
				panic(failure)
			}
			bounds, err = image.Rect(0, 0, 0, 0), fmt.Errorf("%w: disposed image", graphics.ErrInvalid)
		}
	}()

	return source.Bounds(), nil
}

// snapshot imports a borrowed immutable image into a bounded CPU snapshot cache.
// Readback requires a running Ebitengine loop. Snapshots survive processor closure.
func (processor *Processor) snapshot(source *ebiten.Image) (*graphics.Image, error) {
	if !processor.ready() {
		return nil, ErrClosed
	}
	bounds, err := imageBounds(source)
	if err != nil {
		return nil, err
	}
	if entry := processor.sources[source]; entry != nil {
		processor.sourceLRU.MoveToFront(entry.element)

		return entry.snapshot, nil
	}
	const maximumDimension, maximumBytes, channels = 8192, 32 << 20, 4
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 || bounds.Dx() > maximumDimension || bounds.Dy() > maximumDimension ||
		bounds.Dx() > maximumBytes/channels/bounds.Dy() {
		return nil, fmt.Errorf("%w: GPU snapshot size", graphics.ErrInvalid)
	}
	pixels := make([]byte, bounds.Dx()*bounds.Dy()*channels)
	source.ReadPixels(pixels)
	snapshot, err := graphics.NewPixels(bounds.Dx(), bounds.Dy(), pixels)
	if err != nil {
		return nil, fmt.Errorf("snapshot GPU image: %w", err)
	}
	if int64(snapshot.Bytes()) <= processor.limits.SourceBytes {
		for len(processor.sources) >= processor.limits.MaxEntries ||
			processor.stats.SourceBytes > processor.limits.SourceBytes-int64(snapshot.Bytes()) {
			processor.evictSource()
		}
		entry := &sourceEntry{source: source, snapshot: snapshot, element: nil}
		entry.element = processor.sourceLRU.PushFront(entry)
		processor.sources[source] = entry
		processor.stats.SourceBytes += int64(snapshot.Bytes())
	}

	return snapshot, nil
}

// Derive processes a borrowed immutable GPU source, returning a read-only result.
func (processor *Processor) Derive(source *ebiten.Image, operations ...graphics.Operation) (*ebiten.Image, error) {
	snapshot, err := processor.snapshot(source)
	if err != nil {
		return nil, err
	}

	return processor.deriveImage(snapshot, operations...)
}

// deriveImage uses a frozen CPU source and the full ordered operation specification.
func (processor *Processor) deriveImage(
	source *graphics.Image, operations ...graphics.Operation,
) (*ebiten.Image, error) {
	if !processor.ready() {
		return nil, ErrClosed
	}
	if source == nil || len(operations) > maximumOperations {
		return nil, fmt.Errorf("%w: source/operation limit", graphics.ErrInvalid)
	}
	var key derivedKey
	key.source, key.count = source, len(operations)
	copy(key.operations[:], operations)

	return processor.result(key)
}

// Silhouette caches binary occupancy using the same bounded derived-image store.
func (processor *Processor) Silhouette(source *ebiten.Image, threshold byte) (*ebiten.Image, error) {
	snapshot, err := processor.snapshot(source)
	if err != nil {
		return nil, err
	}
	var key derivedKey
	key.source, key.silhouette, key.threshold = snapshot, true, threshold

	return processor.result(key)
}

func (processor *Processor) result(key derivedKey) (*ebiten.Image, error) {
	if cached := processor.derived[key]; cached != nil {
		processor.derivedLRU.MoveToFront(cached.element)

		return cached.image, nil
	}
	result, err := key.source.Apply(key.operations[:key.count]...)
	if err != nil {
		return nil, fmt.Errorf("derive image: %w", err)
	}
	if key.silhouette {
		result = key.source.Silhouette(key.threshold)
	}
	image := ebiten.NewImageFromImage(result.CopyRGBA())
	payload := int64(result.Bytes()) + int64(key.source.Bytes())
	for _, operation := range key.operations[:key.count] {
		payload += operation.RetainedBytes()
	}
	if payload <= processor.limits.DerivedBytes {
		for len(processor.derived) >= processor.limits.MaxEntries ||
			processor.stats.DerivedBytes > processor.limits.DerivedBytes-payload {
			processor.evictDerived()
		}
		entry := &derivedEntry{key: key, image: image, bytes: payload, element: nil}
		entry.element = processor.derivedLRU.PushFront(entry)
		processor.derived[key] = entry
		processor.stats.DerivedBytes += payload
	}

	return image, nil
}

func (processor *Processor) evictSource() {
	element := processor.sourceLRU.Back()
	entry, ok := element.Value.(*sourceEntry)
	if !ok {
		panic("graphics source LRU contains foreign entry")
	}
	delete(processor.sources, entry.source)
	processor.sourceLRU.Remove(element)
	processor.stats.SourceBytes -= int64(entry.snapshot.Bytes())
}

func (processor *Processor) evictDerived() {
	element := processor.derivedLRU.Back()
	entry, ok := element.Value.(*derivedEntry)
	if !ok {
		panic("graphics derived LRU contains foreign entry")
	}
	delete(processor.derived, entry.key)
	processor.derivedLRU.Remove(element)
	processor.stats.DerivedBytes -= entry.bytes
}

// Forget retires a source snapshot after explicit host mutation. Previously
// returned derived borrows keep their frozen pixels and independent Go lifetime.
func (processor *Processor) Forget(source *ebiten.Image) {
	if !processor.ready() {
		return
	}
	entry := processor.sources[source]
	if entry == nil {
		return
	}
	delete(processor.sources, source)
	processor.sourceLRU.Remove(entry.element)
	processor.stats.SourceBytes -= int64(entry.snapshot.Bytes())
	for key, derived := range processor.derived {
		if key.source == entry.snapshot {
			delete(processor.derived, key)
			processor.derivedLRU.Remove(derived.element)
			processor.stats.DerivedBytes -= derived.bytes
		}
	}
}

// Stats returns detached cache-held entry counts and payload.
func (processor *Processor) Stats() Stats {
	result := processor.stats
	result.Sources, result.Derived = len(processor.sources), len(processor.derived)

	return result
}

// Clear releases cache references; read-only returned borrows remain valid.
func (processor *Processor) Clear() {
	clear(processor.sources)
	clear(processor.derived)
	processor.sourceLRU.Init()
	processor.derivedLRU.Init()
	processor.stats.SourceBytes, processor.stats.DerivedBytes = 0, 0
}

// Close is terminal and idempotent; host targets and returned borrows survive.
func (processor *Processor) Close() {
	if processor == nil || processor.closed {
		return
	}
	processor.Clear()
	processor.closed = true
}
