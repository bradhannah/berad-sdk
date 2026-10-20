package rng

import (
	"math"
	"testing"

	"golang.org/x/exp/rand"
)

// These outputs were recorded directly from the published x/exp/rand dependency
// at a4bb9ffd2546, without calling this package or its jump implementation.
func TestKnownPCGOutputs(t *testing.T) {
	fixtures := []struct {
		seed uint64
		want [5]uint64
	}{
		{0, [5]uint64{0xcbf98931523d4eef, 0x4d98b91b8d356870, 0x01070196e695f8f1, 0x703ec840c59f4493, 0xe54954914b3a44fa}},
		{1, [5]uint64{0x34e936394905d167, 0x817c0ef62fe4c731, 0x937987e6e24f5a40, 0x0c0a8307fe226199, 0xf96363568d8bab56}},
		{8675309, [5]uint64{0x201adee541b7384c, 0xe3c7381a73050d46, 0x060619bcc5b200a8, 0x18da8f6c3a51ad5c, 0x4cab04b219a2c057}},
		{0x0123456789abcdef, [5]uint64{0xa12dea8c95158441, 0x242041db494e6da8, 0x2cb3dccd41360faa, 0x4ceae7e3765e3633, 0x65ddd0b932ceeb6b}},
		{math.MaxUint64, [5]uint64{0x54172474d993daad, 0x8bd69346b5cae7a6, 0x1d396726771e6c6b, 0xc052794e7eccc85e, 0xd1046b05929b833b}},
	}
	for _, fixture := range fixtures {
		source := New(fixture.seed)
		for index, expected := range fixture.want {
			if actual := source.source.Uint64(); actual != expected {
				t.Fatalf("seed %#x output %d = %#x, want %#x", fixture.seed, index, actual, expected)
			}
			if actual := Restore(fixture.seed, uint64(index)).source.Uint64(); actual != expected {
				t.Fatalf("restored seed %#x output %d = %#x, want %#x", fixture.seed, index, actual, expected)
			}
		}
		if source.Calls() != uint64(len(fixture.want)) {
			t.Fatalf("Calls() = %d, want %d", source.Calls(), len(fixture.want))
		}
	}
}

func TestRestoreMatchesIterativeDependencyAcrossSeedsAndCalls(t *testing.T) {
	seeds := []uint64{0, 1, 8675309, 0x0123456789abcdef, math.MaxUint64}
	callCounts := []uint64{0, 1, 2, 3, 7, 8, 31, 64, 255, 1024, 10000}
	for _, seed := range seeds {
		for _, calls := range callCounts {
			iterative := rand.NewSource(seed)
			for range calls {
				iterative.Uint64()
			}
			restored := Restore(seed, calls)
			if restored.Seed() != seed || restored.Calls() != calls {
				t.Fatalf("Restore(%d, %d) metadata = %d, %d", seed, calls, restored.Seed(), restored.Calls())
			}
			for output := range 10 {
				if actual, expected := restored.source.Uint64(), iterative.Uint64(); actual != expected {
					t.Fatalf("Restore(%d, %d) output %d = %#x, want %#x", seed, calls, output, actual, expected)
				}
			}
		}
	}
}

func TestRestoreHugeCallCounts(t *testing.T) {
	const seed = uint64(0xfedcba9876543210)
	// Independently calculated states used a big-integer closed-form geometric
	// sum, then x/exp/rand.PCGSource.UnmarshalBinary and Uint64 for these outputs.
	fixtures := []struct {
		calls uint64
		want  uint64
	}{
		{1 << 32, 0xa519d8e5c2d1112c},
		{1 << 63, 0xe3cbfd470103596e},
		{math.MaxUint64, 0x6422365cf588481d},
	}
	for _, fixture := range fixtures {
		restored := Restore(seed, fixture.calls)
		if restored.Calls() != fixture.calls {
			t.Fatalf("Calls() = %d, want %d", restored.Calls(), fixture.calls)
		}
		stepped := Restore(seed, fixture.calls-1024)
		for range 1024 {
			stepped.source.Uint64()
		}
		if actual := restored.source.Uint64(); actual != fixture.want {
			t.Fatalf("jump %d next = %#x, want %#x", fixture.calls, actual, fixture.want)
		}
		if actual := stepped.source.Uint64(); actual != fixture.want {
			t.Fatalf("stepped jump %d next = %#x, want %#x", fixture.calls, actual, fixture.want)
		}
		if restored.Calls() != fixture.calls+1 || stepped.Calls() != fixture.calls+1 {
			t.Fatal("source counter did not preserve uint64 wrap semantics")
		}
	}
}
