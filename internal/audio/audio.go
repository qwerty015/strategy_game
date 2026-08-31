// Package audio plays background music and short sound effects (a
// builder's hammer, a lumberjack's axe, mining) triggered by cmd/game.
// Like internal/assets, it's one of the few packages allowed to touch
// ebiten directly -- see AGENTS.md.
//
// Per the user's explicit instruction, sound files are not baked into
// the binary: only source code and the engine belong in the executable.
// Every file here is read from assets/audio/ beside the executable at
// runtime (see assetDir). A source-tree fallback keeps `go run` and tests
// working from any current directory; this package has
// no embed directive anywhere. A missing file (someone copied just the
// .exe without the assets folder) is logged and that one sound is
// skipped; it must never crash the game.
package audio

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
)

// sampleRate matches the project's only audio.Context -- ebiten allows
// exactly one per process, created once here.
const sampleRate = 44100

var context = audio.NewContext(sampleRate)

// assetDir is normally assets/audio beside strategy_game.exe. The source
// fallback lets development builds find the same files without depending on
// the current working directory.
var assetDir = resolveAssetDir()

func resolveAssetDir() string {
	executableCandidate := filepath.Join("assets", "audio")
	if executable, err := os.Executable(); err == nil {
		executableCandidate = filepath.Join(filepath.Dir(executable), "assets", "audio")
		if directoryExists(executableCandidate) {
			return executableCandidate
		}
	}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		sourceCandidate := filepath.Join(filepath.Dir(thisFile), "..", "..", "assets", "audio")
		if directoryExists(sourceCandidate) {
			return sourceCandidate
		}
	}
	return executableCandidate
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Default volumes: music sits well under the sound effects so hammering/
// chopping/mining reads as the foreground, the loop as background color.
const (
	defaultMusicVolume = 0.25
	defaultSFXVolume   = 0.55
)

var (
	musicVolume = defaultMusicVolume
	sfxVolume   = defaultSFXVolume
)

var (
	chopSFX   [][]byte
	hammerSFX [][]byte
	miningSFX [][]byte

	// Building ambient cues (see cmd/game's tickBuildingAmbientSounds) --
	// one set per building Kind that got a distinct sound in this round.
	// LumberjackHut/QuarryHut/MinerHut aren't here: they already play
	// through chopSFX/miningSFX above, tied to the worker's own state
	// rather than a fixed period.
	scytheSFX        [][]byte // Farm
	millWorkSFX      [][]byte // Mill
	bakerySFX        [][]byte // Bakery
	squishSFX        [][]byte // Winery
	pigOinkSFX       [][]byte // PigFarm
	meatChopSFX      [][]byte // MeatWorkshop
	sawSFX           [][]byte // CarpentryWorkshop
	forgeSFX         [][]byte // Smeltery
	oarSplashSFX     [][]byte // FisherHut
	cartCreakSFX     [][]byte // Warehouse
	tavernChatterSFX [][]byte // Tavern

	// Weather/time-of-day ambient cues -- not tied to any one building,
	// see cmd/game's tickWeatherAmbientSounds/tickDayNightAmbientSounds/
	// tickWindAmbientSounds.
	seagullSFX     [][]byte // camera is looking at water
	waterWavesSFX  [][]byte // camera is looking at water
	cricketSFX     [][]byte // night
	dayAmbienceSFX [][]byte // day
	windSFX        [][]byte // always -- not tied to camera or time of day

	musicPlayer *audio.Player
)

func init() {
	chopSFX = loadOGGSet(assetDir, "chop", 5)
	hammerSFX = loadOGGSet(assetDir, "hammer", 5)
	miningSFX = loadOGGSet(assetDir, "mining", 5)

	scytheSFX = loadOGGSet(assetDir, "scythe", 5)
	millWorkSFX = loadOGGSet(assetDir, "millwork", 3)
	bakerySFX = loadOGGSet(assetDir, "bakery", 2)
	squishSFX = loadOGGSet(assetDir, "squish", 2)
	pigOinkSFX = loadOGGSet(assetDir, "pigoink", 3)
	meatChopSFX = loadOGGSet(assetDir, "meatchop", 4)
	sawSFX = loadOGGSet(assetDir, "saw", 4)
	forgeSFX = loadOGGSet(assetDir, "forge", 5)
	oarSplashSFX = loadOGGSet(assetDir, "oarsplash", 2)
	cartCreakSFX = loadOGGSet(assetDir, "cartcreak", 4)
	tavernChatterSFX = loadOGGSet(assetDir, "tavernchatter", 4)

	seagullSFX = loadOGGSet(assetDir, "seagull", 3)
	waterWavesSFX = loadOGGSet(assetDir, "waterwaves", 3)
	cricketSFX = loadOGGSet(assetDir, "cricket", 1)
	dayAmbienceSFX = loadOGGSet(assetDir, "dayambience", 1)
	windSFX = loadOGGSet(assetDir, "wind", 3)

	musicPlayer = loadMusicLoop(assetDir, "village.wav")
	if musicPlayer != nil {
		musicPlayer.SetVolume(musicVolume)
		musicPlayer.Play()
	}
}

// loadOGGSet reads and decodes name_000.ogg..name_(n-1).ogg from
// dir/sfx/. Each entry is fully decoded into a plain []byte (not kept as
// a live Stream) so PlayChop/PlayHammer/PlayMining can spin up a fresh,
// independent Player per call -- letting the same sound overlap itself
// if two workers happen to trigger it close together, which a single
// shared Player couldn't do. A file that fails to load is logged and
// simply absent from the set; the set can end up smaller than n, or even
// empty, without anything else breaking (see playRandom). dir is a
// parameter (not read from the package assetDir directly) so a test can
// point it at the repo's real assets folder regardless of `go test`'s
// per-package working directory.
func loadOGGSet(dir, name string, n int) [][]byte {
	set := make([][]byte, 0, n)
	for i := range n {
		path := filepath.Join(dir, "sfx", fmt.Sprintf("%s_%03d.ogg", name, i))
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("audio: %s: %v (this sound will be skipped)", path, err)
			continue
		}
		stream, err := vorbis.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
		if err != nil {
			log.Printf("audio: decode %s: %v (this sound will be skipped)", path, err)
			continue
		}
		decoded, err := io.ReadAll(stream)
		if err != nil {
			log.Printf("audio: read %s: %v (this sound will be skipped)", path, err)
			continue
		}
		set = append(set, decoded)
	}
	return set
}

// loadMusicLoop reads and decodes a wav file from dir/music/ and wraps it
// in an ebiten InfiniteLoop, ready to Play(). Returns nil (not a panic)
// if the file is missing or fails to decode -- see this package's doc
// comment on why a missing asset must never crash the game; every Play*
// function and the volume setters already treat a nil player as a silent
// no-op. dir is a parameter for the same testability reason as
// loadOGGSet's.
func loadMusicLoop(dir, name string) *audio.Player {
	path := filepath.Join(dir, "music", name)
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("audio: %s: %v (background music disabled)", path, err)
		return nil
	}
	stream, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(data))
	if err != nil {
		log.Printf("audio: decode %s: %v (background music disabled)", path, err)
		return nil
	}
	loop := audio.NewInfiniteLoop(stream, stream.Length())
	player, err := context.NewPlayer(loop)
	if err != nil {
		log.Printf("audio: create player for %s: %v (background music disabled)", path, err)
		return nil
	}
	return player
}

// PlayChop plays one randomly chosen axe-hit sound. No-op if the sfx
// folder was missing/unreadable at startup.
func PlayChop() { playRandom(chopSFX) }

// PlayHammer plays one randomly chosen construction-hammer sound.
func PlayHammer() { playRandom(hammerSFX) }

// PlayMining plays one randomly chosen pickaxe/mining sound, shared by
// both the quarry and miner professions -- see cmd/game's tickAudioCues.
func PlayMining() { playRandom(miningSFX) }

// PlayScythe plays a Farm's harvest cue.
func PlayScythe() { playRandom(scytheSFX) }

// PlayMillWork plays a Mill's grinding-machinery cue.
func PlayMillWork() { playRandom(millWorkSFX) }

// PlayBakery plays a Bakery's cue.
func PlayBakery() { playRandom(bakerySFX) }

// PlaySquish plays a Winery's grape-stomping cue.
func PlaySquish() { playRandom(squishSFX) }

// PlayPigOink plays a PigFarm's cue.
func PlayPigOink() { playRandom(pigOinkSFX) }

// PlayMeatChop plays a MeatWorkshop's cue.
func PlayMeatChop() { playRandom(meatChopSFX) }

// PlaySaw plays a CarpentryWorkshop's cue.
func PlaySaw() { playRandom(sawSFX) }

// PlayForge plays a Smeltery's cue.
func PlayForge() { playRandom(forgeSFX) }

// PlayOarSplash plays a FisherHut's cue.
func PlayOarSplash() { playRandom(oarSplashSFX) }

// PlayCartCreak plays a Warehouse's cue.
func PlayCartCreak() { playRandom(cartCreakSFX) }

// PlayTavernChatter plays a Tavern's cue.
func PlayTavernChatter() { playRandom(tavernChatterSFX) }

// PlaySeagull plays when the camera's viewport contains a water tile.
func PlaySeagull() { playRandom(seagullSFX) }

// PlayWaterWaves plays alongside PlaySeagull, same trigger -- gentle
// water/wave ambience rather than a bird cry.
func PlayWaterWaves() { playRandom(waterWavesSFX) }

// PlayCricket plays during the night, regardless of camera position.
func PlayCricket() { playRandom(cricketSFX) }

// PlayDayAmbience plays during the day, regardless of camera position.
func PlayDayAmbience() { playRandom(dayAmbienceSFX) }

// PlayWind plays a general ambient wind cue -- not tied to camera
// position, time of day, or any building.
func PlayWind() { playRandom(windSFX) }

func playRandom(set [][]byte) {
	if len(set) == 0 || sfxVolume <= 0 {
		return
	}
	data := set[rand.Intn(len(set))]
	player := context.NewPlayerFromBytes(data)
	player.SetVolume(sfxVolume)
	player.Play()
}

// SetMusicVolume sets the background music volume, 0 (silent, and
// actually paused -- not just inaudible) to 1. Per the user's explicit
// request ("музыку вообще можно отключить"), 0 really stops playback
// rather than just muting it, and raising the volume again resumes from
// where it left off.
func SetMusicVolume(v float64) {
	musicVolume = clamp01(v)
	if musicPlayer == nil {
		return
	}
	musicPlayer.SetVolume(musicVolume)
	switch {
	case musicVolume <= 0:
		musicPlayer.Pause()
	case !musicPlayer.IsPlaying():
		musicPlayer.Play()
	}
}

// MusicVolume returns the current background music volume, for the pause
// menu's volume control to show which level is selected.
func MusicVolume() float64 { return musicVolume }

// SetSFXVolume sets the volume every future PlayChop/PlayHammer/
// PlayMining call uses (existing, already-playing one-shots are
// unaffected -- they're short enough that this never matters in
// practice). 0 suppresses new sound effects entirely rather than playing
// them inaudibly.
func SetSFXVolume(v float64) {
	sfxVolume = clamp01(v)
}

// SFXVolume returns the current sound-effect volume, for the pause menu's
// volume control to show which level is selected.
func SFXVolume() float64 { return sfxVolume }

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
