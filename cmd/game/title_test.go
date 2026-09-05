package main

import (
	"image"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	gamehelp "strategy_game"
	"strategy_game/internal/building"
	"strategy_game/internal/i18n"
	"strategy_game/internal/render"
)

// TestHelpMarkdownImageReferencesExistOnDisk parses the REAL, embedded
// docs/HELP.md (not a hand-built fixture, unlike
// TestParseHelpMarkdownSplitsPagesAndAssets above) and checks every
// ![...](...) reference actually resolves to a file on disk -- sprites
// load from disk at runtime, not go:embed (see AGENTS.md/the project's own
// "no large embedded assets" rule), so a typo'd or renamed filename here
// would only surface as a blank tile the next time someone opens the
// in-game help screen, not a build failure.
func TestHelpMarkdownImageReferencesExistOnDisk(t *testing.T) {
	pages := parseHelpMarkdown(gamehelp.HelpMarkdown)
	if len(pages) == 0 {
		t.Fatal("parsed zero pages out of the real docs/HELP.md")
	}
	checked := 0
	for _, page := range pages {
		for _, line := range page.lines {
			if line.kind != helpLineImage {
				continue
			}
			checked++
			// line.image is a path relative to docs/HELP.md's own
			// directory (docs/), e.g. "../internal/assets/generated/x.png"
			// -- this test's own working directory is cmd/game, two
			// levels below the repo root (cmd/game, not just cmd/), so
			// docs/ itself is "../../docs" from here.
			path := filepath.Join("..", "..", "docs", line.image)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("page %q references missing image %q (resolved to %q): %v", page.title, line.image, path, err)
			}
		}
	}
	if checked == 0 {
		t.Fatal("docs/HELP.md has no image references at all -- suspicious for this project's own established style")
	}
}

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

// TestBuildVersionIsVisibleReleaseMarker checks BuildVersion's FORMAT
// (ver_<version>_alpha_build_<YYYY.DD.MM>), not an exact hardcoded string --
// a real gap found twice in a row: this test previously pinned the exact
// literal, which went stale the moment BuildVersion was bumped for a new
// build/release-notes pass (once from 0.1 to 0.15, then again just from the
// date advancing to 2026.05.09), breaking the whole suite over a change
// that was never actually a bug. The naming convention itself (see
// release_notes/README.md) is what's worth locking in, not today's value.
func TestBuildVersionIsVisibleReleaseMarker(t *testing.T) {
	pattern := regexp.MustCompile(`^ver_\d+(\.\d+)?_alpha_build_\d{4}\.\d{2}\.\d{2}$`)
	if !pattern.MatchString(BuildVersion) {
		t.Fatalf("BuildVersion = %q, want it to match %s", BuildVersion, pattern)
	}
}

func TestModeSelectActionAt(t *testing.T) {
	rects := modeSelectButtonRects(1280, 720)
	for index, rect := range rects {
		mode, ok := modeSelectActionAt(rect.Min.X+1, rect.Min.Y+1, 1280, 720)
		if !ok || mode != gameMode(index) {
			t.Fatalf("card %d: mode=%d ok=%v", index, mode, ok)
		}
	}
	if _, ok := modeSelectActionAt(0, 0, 1280, 720); ok {
		t.Fatal("empty point unexpectedly has a mode")
	}
}

func TestDifficultySelectActionAt(t *testing.T) {
	rects := difficultyButtonRects(1280, 720)
	want := [3]aiDifficulty{AIEasy, AINormal, AIHard}
	for index, rect := range rects {
		difficulty, ok := difficultySelectActionAt(rect.Min.X+1, rect.Min.Y+1, 1280, 720)
		if !ok || difficulty != want[index] {
			t.Fatalf("card %d: difficulty=%v ok=%v, want %v", index, difficulty, ok, want[index])
		}
	}
	if _, ok := difficultySelectActionAt(0, 0, 1280, 720); ok {
		t.Fatal("empty point unexpectedly has a difficulty")
	}
}

// TestNewGameFlow_FreeMapStaysSinglePlayer locks in that picking "Свободная
// карта" from the mode-select screen behaves exactly like the old direct
// "Новая игра" button always did: an ordinary single-player game, g.ais empty.
func TestNewGameFlow_FreeMapStaysSinglePlayer(t *testing.T) {
	g := NewGame()
	g.screen = screenTitle
	g.enterModeSelect()
	if g.screen != screenModeSelect {
		t.Fatalf("screen = %v, want screenModeSelect", g.screen)
	}
	g.startFreeMapGame()
	if len(g.ais) != 0 {
		t.Fatal("free map game unexpectedly has an AI faction")
	}
	if g.buildings == nil {
		t.Fatal("free map game has no buildings")
	}
}

// TestNewGameFlow_DuelModeStartsASecondFaction locks in the other half of
// the goal this screen exists for: picking "1×1 с ИИ" then a difficulty
// must actually reach a playable duel game (g.ais set, g.screen back to
// screenPlay) -- before this screen existed there was no way to reach
// newDuelGame from the running application at all.
func TestNewGameFlow_DuelModeStartsASecondFaction(t *testing.T) {
	g := NewGame()
	g.screen = screenTitle
	g.enterModeSelect()
	g.screen = screenDifficultySelect
	g.startDuelGame([]aiDifficulty{AIHard})
	if len(g.ais) == 0 {
		t.Fatal("duel game has no AI faction")
	}
	if g.ais[0].brain.difficulty != AIHard {
		t.Fatalf("ai difficulty = %v, want AIHard", g.ais[0].brain.difficulty)
	}
}

// TestNewGameFlow_DuelModeWithMultipleOpponentsPicksOneDifficultyEach
// covers "4х4" (always maxDuelOpponents == 3 bots, per the user's
// explicit "оставь только режим 'Свободный' и '4х4'") end to end: per the
// user's separate, earlier "подумай над выбором уровня сложности для
// каждого противника", each of the 3 bots gets its own difficulty pick,
// not one shared difficulty for the whole match. Drives the actual
// accumulation state (g.duelDifficulties) the real screenDifficultySelect
// flow builds up one visit at a time, rather than simulating live cursor
// clicks (same convention as TestNewGameFlow_DuelModeStartsASecondFaction,
// which already sets g.screen directly rather than clicking through it).
func TestNewGameFlow_DuelModeWithMultipleOpponentsPicksOneDifficultyEach(t *testing.T) {
	g := NewGame()
	g.screen = screenTitle
	g.enterModeSelect()
	g.duelOpponentCount = maxDuelOpponents
	g.duelDifficulties = nil
	g.screen = screenDifficultySelect

	picks := []aiDifficulty{AIEasy, AINormal, AIHard}
	for _, difficulty := range picks {
		g.duelDifficulties = append(g.duelDifficulties, difficulty)
		if len(g.duelDifficulties) >= g.duelOpponentCount {
			g.startDuelGame(g.duelDifficulties)
		}
	}

	if len(g.ais) != 3 {
		t.Fatalf("len(g.ais) = %d, want 3", len(g.ais))
	}
	for i, want := range picks {
		if g.ais[i].brain.difficulty != want {
			t.Fatalf("bot %d difficulty = %v, want %v (picked in order)", i, g.ais[i].brain.difficulty, want)
		}
	}
}

// TestSaveGame_SucceedsDuringADuelGame locks in the opposite of what this
// test used to assert: an explicit user report ("а я не могу сохранить
// игру если играю с ботом?") turned "duel saves are refused" from a
// deliberate guard into a missing feature, so saveGame no longer refuses
// -- it serializes g.ais' full faction state (see buildSaveState's
// IsDuelGame/AI* fields and restoreFaction), same as a free-map game.
func TestSaveGame_SucceedsDuringADuelGame(t *testing.T) {
	g := newDuelGame([]aiDifficulty{AINormal})
	tmp := t.TempDir() + "/slot1.json"
	if err := g.saveGame(tmp, "test"); err != nil {
		t.Fatalf("saveGame unexpectedly failed for a duel game: %v", err)
	}

	free := NewGame()
	tmp2 := t.TempDir() + "/slot1.json"
	if err := free.saveGame(tmp2, "test"); err != nil {
		t.Fatalf("saveGame unexpectedly failed for a free-map game: %v", err)
	}
}

// TestEnterModeSelect_BackFromPauseResumesThePausedGame is the mode-select
// screen's other real entry point: the Esc pause menu's "Новая игра" ->
// confirm -> mode-select -> "Назад" must resume the actual paused game,
// not strand the player on the title screen or silently discard nothing
// while showing the wrong screen.
func TestEnterModeSelect_BackFromPauseResumesThePausedGame(t *testing.T) {
	g := NewGame()
	g.screen = screenPlay
	g.paused = true
	g.enterModeSelect()
	if g.screen != screenModeSelect || g.paused {
		t.Fatalf("after enterModeSelect: screen=%v paused=%v", g.screen, g.paused)
	}
	g.leaveModeSelect()
	if g.screen != screenPlay || !g.paused {
		t.Fatalf("after leaveModeSelect: screen=%v paused=%v, want screenPlay+paused", g.screen, g.paused)
	}
}
