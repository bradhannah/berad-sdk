module example.com/berad-audio-tools-consumer

go 1.27.0

require github.com/bradhannah/berad-sdk/tools/berad-audio v0.0.0

// Supplied only in the checker's isolated consumer layout.
replace github.com/bradhannah/berad-sdk/tools/berad-audio => ../library
