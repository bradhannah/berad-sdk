package ebitengine_test

import (
	"bytes"
	"image/color"
	"math"
	"testing"

	backend "github.com/bradhannah/berad-sdk/modules/berad-graphics/backend/ebitengine"
	"github.com/hajimehoshi/ebiten/v2"
)

func TestColourAdjustmentIdentityTransparencyAndMeaningfulZero(t *testing.T) {
	source := ebiten.NewImage(1, 1)
	defer source.Deallocate()
	source.Fill(color.RGBA{R: 80, G: 20, B: 40, A: 128})
	target := ebiten.NewImage(1, 1)
	defer target.Deallocate()
	adjustment := backend.DefaultColourAdjustment()
	if err := backend.DrawAdjusted(target, source, adjustment, ebiten.DrawImageOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pixels(source), pixels(target)) {
		t.Fatal("identity changed pixels", pixels(target))
	}
	adjustment.Value = 0
	target.Clear()
	if err := backend.DrawAdjusted(target, source, adjustment, ebiten.DrawImageOptions{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pixels(target), []byte{0, 0, 0, 128}) {
		t.Fatal("zero value is not black with preserved alpha", pixels(target))
	}
	adjustment.Value = 1
	adjustment.Saturation = 0
	target.Clear()
	if err := backend.DrawAdjusted(target, source, adjustment, ebiten.DrawImageOptions{}); err != nil {
		t.Fatal(err)
	}
	value := pixels(target)
	if value[0] != value[1] || value[1] != value[2] || value[3] != 128 {
		t.Fatal("desaturation changed alpha or retained chroma", value)
	}
	adjustment.HueRadians = math.NaN()
	if err := backend.DrawAdjusted(target, source, adjustment, ebiten.DrawImageOptions{}); err == nil {
		t.Fatal("NaN accepted")
	}
}
