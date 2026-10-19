module example.com/berad-audio-consumer

go 1.27.0

require github.com/bradhannah/berad-sdk/modules/berad-audio v0.0.0

// Local substitution belongs only to this isolated test consumer. The gate
// places a copy of the reusable module here; the published game has no replace.
replace github.com/bradhannah/berad-sdk/modules/berad-audio => ../library
