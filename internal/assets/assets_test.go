package assets

import (
	"image"
	"image/color"
	"testing"
)

func TestRemoveDetachedAlphaComponents(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for y := 1; y <= 3; y++ {
		for x := 0; x <= 2; x++ {
			img.Set(x, y, color.White)
		}
	}
	img.Set(7, 0, color.White)
	img.Set(7, 1, color.White)

	removeDetachedAlphaComponents(img, 3)
	if got := img.NRGBAAt(7, 0).A; got != 0 {
		t.Fatalf("detached fragment alpha = %d, want 0", got)
	}
	if got := img.NRGBAAt(1, 2).A; got == 0 {
		t.Fatal("main sprite component was removed")
	}
}

func TestRoundedRoadEntranceSoftensOnlyRequestedCorner(t *testing.T) {
	const northWest uint8 = 1
	if got := roundedRoadCornerAlpha(northWest, 0, 0); got != 0 {
		t.Fatalf("requested top-left corner alpha = %d, want 0", got)
	}
	if got := roundedRoadCornerAlpha(northWest, TileSize-1, 0); got != 255 {
		t.Fatalf("unrequested top-right corner alpha = %d, want 255", got)
	}
	if got := roundedRoadCornerAlpha(northWest, TileSize/2, 0); got != 255 {
		t.Fatalf("top edge centre alpha = %d, want 255", got)
	}
}
