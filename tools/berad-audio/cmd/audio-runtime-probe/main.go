// Command audio-runtime-probe owns a native Ebitengine loop for audio diagnostics.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bradhannah/berad-sdk/tools/berad-audio/project"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/runtimeprobe"
)

func main() {
	const maxLibraryBytes = 512 << 20
	descriptor := flag.String("project", "", "Explicit tool project descriptor")
	cue := flag.String("cue", "", "Cue to probe")
	soak := flag.Bool("soak", false, "Run muted one-minute lifecycle/heap probe")
	flag.Parse()
	if *descriptor == "" || *cue == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "require -project and -cue")
		os.Exit(1)
	}
	doc, _, err := project.Open(*descriptor)
	if err == nil {
		library, failure := doc.RuntimeLibrary(maxLibraryBytes)
		err = failure
		if err == nil {
			err = runtimeprobe.Run(library, *cue, *soak, os.Stdout)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
