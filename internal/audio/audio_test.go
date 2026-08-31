package audio

import (
	"path/filepath"
	"runtime"
	"testing"
)

// repoAssetDir resolves assets/audio/ relative to this test file's own
// location rather than the current working directory: `go test` runs
// with the package directory as its CWD, not the repo root, so the
// package-level assetDir (resolved once during startup, next to the executable)
// cannot be used here directly.
func repoAssetDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "assets", "audio")
}

// TestVolumeSettersClampAndReport covers the pause menu's volume control
// contract: values outside 0..1 are clamped, and the getters reflect
// whatever was actually set (used to highlight the selected level).
func TestVolumeSettersClampAndReport(t *testing.T) {
	defer func() {
		SetMusicVolume(defaultMusicVolume)
		SetSFXVolume(defaultSFXVolume)
	}()

	SetMusicVolume(0.5)
	if got := MusicVolume(); got != 0.5 {
		t.Fatalf("MusicVolume() = %v, want 0.5", got)
	}
	SetMusicVolume(-1)
	if got := MusicVolume(); got != 0 {
		t.Fatalf("MusicVolume() after SetMusicVolume(-1) = %v, want 0 (clamped)", got)
	}
	SetMusicVolume(2)
	if got := MusicVolume(); got != 1 {
		t.Fatalf("MusicVolume() after SetMusicVolume(2) = %v, want 1 (clamped)", got)
	}

	SetSFXVolume(0.75)
	if got := SFXVolume(); got != 0.75 {
		t.Fatalf("SFXVolume() = %v, want 0.75", got)
	}
	SetSFXVolume(-5)
	if got := SFXVolume(); got != 0 {
		t.Fatalf("SFXVolume() after SetSFXVolume(-5) = %v, want 0 (clamped)", got)
	}
}

// TestPlayFunctionsDoNotPanicAtZeroVolume covers the "0 suppresses new
// sound effects entirely" contract from SetSFXVolume's doc comment --
// must return immediately, not attempt to build and play a silent
// Player.
func TestPlayFunctionsDoNotPanicAtZeroVolume(t *testing.T) {
	defer SetSFXVolume(defaultSFXVolume)
	SetSFXVolume(0)
	PlayChop()
	PlayHammer()
	PlayMining()
	PlayScythe()
	PlayMillWork()
	PlayBakery()
	PlaySquish()
	PlayPigOink()
	PlayMeatChop()
	PlaySaw()
	PlayForge()
	PlayOarSplash()
	PlayCartCreak()
	PlayTavernChatter()
	PlaySeagull()
	PlayWaterWaves()
	PlayCricket()
	PlayDayAmbience()
	PlayWind()
}

// TestSetSFXVolumeZeroStopsAlreadyPlayingSounds covers the user's explicit
// "если громкость в 0 - полная тишина!" requirement: dropping SFX volume
// to 0 must immediately silence a sound that's already mid-playback, not
// just suppress future Play* calls -- otherwise a long clip started a
// moment before the click keeps playing at its old volume until it
// finishes.
func TestSetSFXVolumeZeroStopsAlreadyPlayingSounds(t *testing.T) {
	defer SetSFXVolume(defaultSFXVolume)
	if len(chopSFX) == 0 {
		t.Skip("chopSFX is empty -- real assets/audio/sfx/chop_*.ogg not available in this environment")
	}

	SetSFXVolume(defaultSFXVolume)
	PlayChop()
	if len(activeSFX) == 0 || !activeSFX[len(activeSFX)-1].IsPlaying() {
		t.Fatal("PlayChop() at nonzero volume did not start a tracked, playing player -- test setup is broken")
	}

	SetSFXVolume(0)
	for _, p := range activeSFX {
		if p.IsPlaying() {
			t.Error("a player from before SetSFXVolume(0) is still playing")
		}
	}
	if len(activeSFX) != 0 {
		t.Errorf("activeSFX after SetSFXVolume(0) = %d entries, want 0", len(activeSFX))
	}
}

// TestLoadOGGSetSkipsMissingFilesInsteadOfPanicking is the core contract
// from this package's doc comment: assets live on disk, not go:embed'd,
// so a missing folder/file (e.g. the .exe was copied without assets/)
// must degrade gracefully, never crash.
func TestLoadOGGSetSkipsMissingFilesInsteadOfPanicking(t *testing.T) {
	set := loadOGGSet(repoAssetDir(t), "this-sound-does-not-exist", 5)
	if len(set) != 0 {
		t.Fatalf("loadOGGSet for a nonexistent sound = %d entries, want 0", len(set))
	}
}

// TestLoadMusicLoopReturnsNilInsteadOfPanickingWhenMissing mirrors the
// above for the background music loader.
func TestLoadMusicLoopReturnsNilInsteadOfPanickingWhenMissing(t *testing.T) {
	if p := loadMusicLoop(repoAssetDir(t), "this-track-does-not-exist.wav"); p != nil {
		t.Fatal("loadMusicLoop for a nonexistent file returned a non-nil player")
	}
}

// TestRealAssetsLoadSuccessfully is a sanity check against the actual
// files shipped in assets/audio/ (see LICENSES.md) -- catches a bad
// rename/copy of the real sound files, not just the missing-file path.
// Loads them fresh here (not via the package-level chopSFX/musicPlayer
// vars, which init() already populated -- successfully or not -- against
// go test's own working directory before this test ever runs).
func TestRealAssetsLoadSuccessfully(t *testing.T) {
	dir := repoAssetDir(t)
	sets := []struct {
		name string
		n    int
	}{
		{"chop", 5}, {"hammer", 5}, {"mining", 5},
		{"scythe", 5}, {"millwork", 3}, {"bakery", 2}, {"squish", 2},
		{"pigoink", 3}, {"meatchop", 4}, {"saw", 4}, {"forge", 5},
		{"oarsplash", 2}, {"cartcreak", 4}, {"tavernchatter", 4},
		{"seagull", 3}, {"waterwaves", 3}, {"cricket", 1}, {"dayambience", 1},
		{"wind", 3},
	}
	for _, set := range sets {
		if got := loadOGGSet(dir, set.name, set.n); len(got) != set.n {
			t.Errorf("loadOGGSet(%s) = %d of %d real assets/audio/sfx/%s_*.ogg files", set.name, len(got), set.n, set.name)
		}
	}
	if p := loadMusicLoop(dir, "village.wav"); p == nil {
		t.Error("loadMusicLoop(village.wav) returned nil -- assets/audio/music/village.wav did not load")
	}
}
