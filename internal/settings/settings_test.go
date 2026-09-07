package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingSettingsDoesNotCreateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	c, err := Load(path)
	if err != nil || c != Defaults() {
		t.Fatalf("%+v, %v", c, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("load created settings")
	}
}

func TestSettingsRoundTripAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	for _, want := range []Config{{"en", 0, 0.75}, {"ru", 1, 0}} {
		if err := Save(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got != want {
			t.Fatalf("got %+v, %v; want %+v", got, err, want)
		}
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings-*.tmp"))
	if len(files) != 0 {
		t.Fatal("temporary settings leaked")
	}
}

func TestPartialAndInvalidSettings(t *testing.T) {
	for _, tc := range []struct {
		text string
		want Config
		bad  bool
	}{
		{`{"language":"en"}`, Config{"en", DefaultMusicVolume, DefaultSFXVolume}, false},
		{`{"music_volume":0,"sfx_volume":0}`, Config{"ru", 0, 0}, false},
		{`{"language":"xx","music_volume":-1,"sfx_volume":2}`, Defaults(), false},
		{`{"language":`, Defaults(), true},
	} {
		path := filepath.Join(t.TempDir(), FileName)
		if err := os.WriteFile(path, []byte(tc.text), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if got != tc.want || (err != nil) != tc.bad {
			t.Fatalf("%s: %+v, %v", tc.text, got, err)
		}
	}
}

func TestInvalidSaveKeepsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := Save(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Config{"xx", 1, 1}); err == nil {
		t.Fatal("invalid settings accepted")
	}
	got, _ := Load(path)
	if got != Defaults() {
		t.Fatal("existing settings overwritten")
	}
}

func TestSettingsPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module strategy_game\n"), 0600); err != nil {
		t.Fatal(err)
	}
	devExe := filepath.Join(t.TempDir(), "go-build123", "b001", "exe", "game.exe")
	for _, cwd := range []string{root, filepath.Join(root, "cmd", "game")} {
		if got := PathFor(devExe, cwd); got != filepath.Join(root, FileName) {
			t.Fatalf("go run: %s", got)
		}
	}
	installed := filepath.Join(t.TempDir(), "installed", "strategy_game.exe")
	if got := PathFor(installed, root); got != filepath.Join(filepath.Dir(installed), FileName) {
		t.Fatalf("installed executable used source settings: %s", got)
	}
}
