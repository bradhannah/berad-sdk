package rng_test

import (
	"fmt"

	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func ExampleRestore() {
	source := rng.New(8675309)
	fmt.Println(source.Intn(1000))
	seed, calls := source.Seed(), source.Calls()
	restored := rng.Restore(seed, calls)
	fmt.Println(source.Intn(1000) == restored.Intn(1000))
	// Output:
	// 604
	// true
}
