// Command audio-probe checks or auditions one explicitly selected local recording.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/bradhannah/berad-sdk/tools/berad-audio/audition"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("audio-probe", flag.ContinueOnError)
	file := flags.String("file", "", "Explicit WAV, MP3 or OGG source path")
	check := flags.Bool("check", false, "Analyze without opening a native device")
	duration := flags.Duration("duration", 2*time.Second, "Native probe duration (positive, at most 1m)")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parse probe options: %w", err)
	}
	if *file == "" || flags.NArg() != 0 || *duration <= 0 || *duration > time.Minute {
		return fmt.Errorf("%w: supply -file and a duration in (0,1m] without positional arguments", audition.ErrInvalidVoice)
	}
	clip, err := authoring.LoadRecordingContext(ctx, *file, ebitengine.Decode)
	if err != nil {
		return err
	}
	if *check {
		_, err := fmt.Fprintf(output, "Playable source: %d ms; SHA256 %s (listening review is separate)\n", clip.DurationMs(), clip.Fingerprint())
		return err
	}
	voice := audition.NewVoice()
	if err := voice.Play(clip, nil, false, 0.2); err != nil {
		return err
	}
	timer := time.NewTimer(*duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.Join(ctx.Err(), voice.Close())
	case <-timer.C:
	}
	position, failure := voice.PositionMs(), voice.Failure()
	if err := errors.Join(failure, voice.Close()); err != nil {
		return err
	}
	if position <= 0 {
		return fmt.Errorf("%w: native playback did not advance", audition.ErrInvalidVoice)
	}
	_, err = fmt.Fprintf(output, "Standalone playback advanced to %d ms\n", position)
	return err
}
