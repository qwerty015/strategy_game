package audio

import (
	"path/filepath"
	"runtime"
	"testing"
)

// repoAssetDir resolves assets/audio/ relative to this test file's own
// location rather than the current working directory: `go test` runs
// with the package directory as its CWD, not the repo root, so the
// package-level assetDir constant (meant to be resolved from wherever the
// game binary is actually launched) can't be used here directly.
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
	if got := loadOGGSet(dir, "chop", 5); len(got) != 5 {
		t.Errorf("loadOGGSet(chop) = %d of 5 real assets/audio/sfx/chop_*.ogg files", len(got))
	}
	if got := loadOGGSet(dir, "hammer", 5); len(got) != 5 {
		t.Errorf("loadOGGSet(hammer) = %d of 5 real assets/audio/sfx/hammer_*.ogg files", len(got))
	}
	if got := loadOGGSet(dir, "mining", 5); len(got) != 5 {
		t.Errorf("loadOGGSet(mining) = %d of 5 real assets/audio/sfx/mining_*.ogg files", len(got))
	}
	if p := loadMusicLoop(dir, "village.wav"); p == nil {
		t.Error("loadMusicLoop(village.wav) returned nil -- assets/audio/music/village.wav did not load")
	}
}
