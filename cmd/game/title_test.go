package main

import (
	"image"
	"testing"

	"strategy_game/internal/building"
	"strategy_game/internal/i18n"
	"strategy_game/internal/render"
)

func TestParseHelpMarkdownSplitsPagesAndAssets(t *testing.T) {
	pages := parseHelpMarkdown(`# Document title

## First page

Plain **text**.

![Farm](../internal/assets/generated/building_farm.png)

## Second page

### Detail

- A list item.
`)
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}
	if pages[0].title != "First page" || len(pages[0].lines) != 2 {
		t.Fatalf("first page = %#v", pages[0])
	}
	if got := pages[0].lines[0]; got.kind != helpLineText || got.text != "Plain text." {
		t.Fatalf("first line = %#v", got)
	}
	if got := pages[0].lines[1]; got.kind != helpLineImage || got.image != "../internal/assets/generated/building_farm.png" {
		t.Fatalf("image line = %#v", got)
	}
	if pages[1].title != "Second page" || pages[1].lines[0].kind != helpLineHeading {
		t.Fatalf("second page = %#v", pages[1])
	}
}

func TestTitlePresentationUsesTwoHundredPercentZoom(t *testing.T) {
	if titleMapZoom != 2.00 {
		t.Fatalf("title map zoom = %.2f, want 2.00", titleMapZoom)
	}
}

func TestTitleActionAt(t *testing.T) {
	rects := titleButtonRects(1280, 720)
	for index, rect := range rects {
		action, ok := titleActionAt(rect.Min.X+1, rect.Min.Y+1, 1280, 720)
		if !ok || action != titleAction(index) {
			t.Fatalf("button %d: action=%d ok=%v", index, action, ok)
		}
	}
	if _, ok := titleActionAt(0, 0, 1280, 720); ok {
		t.Fatal("empty point unexpectedly has an action")
	}
}

func TestFrontScreenDimensionsPreferRenderedFullscreenSize(t *testing.T) {
	game := NewGame()
	game.frontWidth = 1920
	game.frontHeight = 1080

	width, height := game.frontScreenDimensions()
	if width != 1920 || height != 1080 {
		t.Fatalf("front-screen dimensions = %dx%d, want rendered 1920x1080", width, height)
	}
}

func TestTitleShowcaseIsSelfContainedAndCoversAllDistricts(t *testing.T) {
	game := &Game{}
	game.populateTitleTown()
	if game.grid == nil || game.grid.Width != titleMapWidth || game.grid.Height != titleMapHeight {
		t.Fatalf("showcase grid = %#v, want %dx%d", game.grid, titleMapWidth, titleMapHeight)
	}
	counts := make(map[building.Kind]int)
	for _, current := range game.buildings {
		counts[current.Kind]++
	}
	for _, kind := range []building.Kind{
		building.Farm, building.Winery, building.Warehouse, building.Tavern,
		building.LumberjackHut, building.QuarryHut, building.MinerHut,
		building.Smeltery, building.FisherHut, building.Road,
	} {
		if counts[kind] == 0 {
			t.Fatalf("showcase lacks %v", kind)
		}
	}
}

func TestTitleShowcaseOpensOnBuildingsWithVisibleWorkers(t *testing.T) {
	game := newGameWithSize(titleMapWidth, titleMapHeight)
	game.populateTitleTown()
	game.camera.Scale = titleMapZoom
	game.camera.SetViewport(0, 0, screenWidth, screenHeight)
	game.positionTitleCamera()

	visible := game.camera.VisibleTileBounds(1)
	standing := 0
	for _, current := range game.buildings {
		if current.Kind != building.Road && visible.Intersects(current.X, current.Y, building.Types[current.Kind].Footprint) {
			standing++
		}
	}
	if standing == 0 {
		t.Fatal("opening title camera sees no standing buildings")
	}
	if len(game.vills.Villagers) == 0 || len(game.logi.Serfs) == 0 {
		t.Fatalf("title workers: villagers=%d serfs=%d; want both groups visible", len(game.vills.Villagers), len(game.logi.Serfs))
	}
}
func TestTitleCameraAxisPingPongsInsideMap(t *testing.T) {
	worldPixels := float64(titleMapWidth * render.TileSize)
	viewPixels := 960.0
	left := titleCameraAxis(0, worldPixels, viewPixels, 0)
	right := titleCameraAxis(titleCameraPeriod/2, worldPixels, viewPixels, 0)
	back := titleCameraAxis(titleCameraPeriod, worldPixels, viewPixels, 0)
	maximum := worldPixels - viewPixels - float64(titleCameraMarginTiles*render.TileSize)
	if left < 0 || right > maximum || right <= left {
		t.Fatalf("camera sweep = left %.1f, right %.1f, maximum %.1f", left, right, maximum)
	}
	if back != left {
		t.Fatalf("camera after a full cycle = %.1f, want %.1f", back, left)
	}
}

func TestTitleLanguageChoicesCoverBothLanguages(t *testing.T) {
	russian, english := titleLanguageRects(1280, 720)
	for _, tc := range []struct {
		name string
		rect image.Rectangle
		want i18n.Lang
	}{
		{"russian", russian, i18n.RU},
		{"english", english, i18n.EN},
	} {
		got, ok := titleLanguageAt(tc.rect.Min.X+tc.rect.Dx()/2, tc.rect.Min.Y+tc.rect.Dy()/2, 1280, 720)
		if !ok || got != tc.want {
			t.Fatalf("%s language choice = %q, %v; want %q, true", tc.name, got, ok, tc.want)
		}
	}
	if _, ok := titleLanguageAt(0, 0, 1280, 720); ok {
		t.Fatal("empty point unexpectedly resolved to a language")
	}
}

func TestBuildVersionIsVisibleReleaseMarker(t *testing.T) {
	const want = "ver_0.1_alpha_build_2026.31.08"
	if BuildVersion != want {
		t.Fatalf("BuildVersion = %q, want %q", BuildVersion, want)
	}
}
