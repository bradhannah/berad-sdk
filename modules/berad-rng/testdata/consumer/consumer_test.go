package consumer_test

import (
	"encoding/json"
	"slices"
	"testing"

	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

// A host-defined checkpoint deliberately has no SDK serialization dependency.
type checkpoint struct {
	Seed  uint64 `json:"seed"`
	Draws uint64 `json:"draws"`
}

func TestSyntheticDeckResumesFromHostCheckpoint(t *testing.T) {
	source := rng.New(8675309)
	deck := []string{"copper", "willow", "cloud", "harbor", "pebble"}
	source.Intn(17)
	source.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	encoded, err := json.Marshal(checkpoint{Seed: source.Seed(), Draws: source.Calls()})
	if err != nil {
		t.Fatal(err)
	}
	var saved checkpoint
	if err := json.Unmarshal(encoded, &saved); err != nil {
		t.Fatal(err)
	}
	resumed := rng.Restore(saved.Seed, saved.Draws)
	originalDeck, restoredDeck := slices.Clone(deck), slices.Clone(deck)
	source.Shuffle(len(originalDeck), func(i, j int) { originalDeck[i], originalDeck[j] = originalDeck[j], originalDeck[i] })
	resumed.Shuffle(len(restoredDeck), func(i, j int) { restoredDeck[i], restoredDeck[j] = restoredDeck[j], restoredDeck[i] })
	if !slices.Equal(originalDeck, restoredDeck) || source.Intn(1000) != resumed.Intn(1000) || source.Calls() != resumed.Calls() {
		t.Fatal("host checkpoint did not resume the deck and next draw")
	}
	borrowed := source
	source.Reset(saved.Seed)
	if borrowed != source || borrowed.Calls() != 0 || borrowed.Intn(1000) != 604 {
		t.Fatal("host borrower did not observe reset to the known first draw")
	}
}
