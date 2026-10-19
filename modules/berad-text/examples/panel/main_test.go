package main

import (
	"image"
	"testing"

	text "github.com/bradhannah/berad-sdk/modules/berad-text"
	"golang.org/x/image/font/gofont/goregular"
)

func TestHostRoutesBeforeTextHitAndKeepsCallbacksOutsideSDK(t *testing.T) {
	source, err := text.ParseFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	font, err := source.NewFont(text.FontOptions{Size: 24})
	if err != nil {
		t.Fatal(err)
	}
	content, err := text.NewContent([]text.Span{{Text: "Activate", Font: font, Tag: "target"}})
	if err != nil {
		t.Fatal(err)
	}
	layout, err := text.NewLayout(content, text.LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	origin := image.Pt(20, 30)
	point := image.Pt(23, 35)
	if got := annotationAt(layout, point, origin, image.Rectangle{}); got != "target" {
		t.Fatalf("uncovered hit: %q", got)
	}
	if got := annotationAt(layout, point, origin, image.Rect(0, 0, 100, 100)); got != "" {
		t.Fatalf("click through blocker: %q", got)
	}
	if got := annotationAt(layout, image.Pt(999, 999), origin, image.Rectangle{}); got != "" {
		t.Fatalf("outside hit: %q", got)
	}
}
