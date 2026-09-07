// Package settings stores local preferences independently of game saves.
package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const FileName = "settings.json"
const DefaultMusicVolume = 0.25
const DefaultSFXVolume = 0.55

type Config struct {
	Language    string  `json:"language"`
	MusicVolume float64 `json:"music_volume"`
	SFXVolume   float64 `json:"sfx_volume"`
}

func Defaults() Config {
	return Config{Language: "ru", MusicVolume: DefaultMusicVolume, SFXVolume: DefaultSFXVolume}
}

// Go's temporary/cached executable must not own durable preferences.
// Installed executables always use their own directory, regardless of cwd.
func PathFor(executable, workingDirectory string) string {
	development := executable == ""
	for dir := filepath.Dir(executable); dir != "."; dir = filepath.Dir(dir) {
		if strings.HasPrefix(strings.ToLower(filepath.Base(dir)), "go-build") {
			development = true
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	if development {
		for dir := workingDirectory; ; dir = filepath.Dir(dir) {
			data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
			if err == nil && strings.Contains(string(data), "module strategy_game") {
				return filepath.Join(dir, FileName)
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
		return filepath.Join(workingDirectory, FileName)
	}
	return filepath.Join(filepath.Dir(executable), FileName)
}

var startupPath = func() string {
	executable, _ := os.Executable()
	wd, _ := os.Getwd()
	return PathFor(executable, wd)
}()
var startup = func() Config {
	c, err := Load(startupPath)
	if err != nil {
		log.Printf("settings: %v", err)
	}
	return c
}()

func Path() string    { return startupPath }
func Startup() Config { return startup }

func validVolume(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }

func Load(path string) (Config, error) {
	c := Defaults()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	// Pointer fields distinguish a saved zero volume from an absent field.
	var stored struct {
		Language    *string  `json:"language"`
		MusicVolume *float64 `json:"music_volume"`
		SFXVolume   *float64 `json:"sfx_volume"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return c, err
	}
	if stored.Language != nil && (*stored.Language == "ru" || *stored.Language == "en") {
		c.Language = *stored.Language
	}
	if stored.MusicVolume != nil && validVolume(*stored.MusicVolume) {
		c.MusicVolume = *stored.MusicVolume
	}
	if stored.SFXVolume != nil && validVolume(*stored.SFXVolume) {
		c.SFXVolume = *stored.SFXVolume
	}
	return c, nil
}

// Save writes atomically in the same directory so a failed write cannot
// truncate an existing configuration. Reading preferences never creates it.
func Save(path string, c Config) error {
	if (c.Language != "ru" && c.Language != "en") || !validVolume(c.MusicVolume) || !validVolume(c.SFXVolume) {
		return fmt.Errorf("invalid preferences")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(append(data, '\n'))
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return writeErr
	}
	return os.Rename(f.Name(), path)
}
