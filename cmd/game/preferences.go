package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"log"
	"strategy_game/internal/audio"
	"strategy_game/internal/i18n"
	"strategy_game/internal/settings"
)

func (g *Game) savePreferences() {
	c := settings.Config{Language: string(i18n.Current()), MusicVolume: audio.MusicVolume(), SFXVolume: audio.SFXVolume()}
	if err := settings.Save(settings.Path(), c); err != nil {
		log.Printf("settings: %v", err)
		g.statusMsg = i18n.T().SettingsSaveFailed
	}
}

func (g *Game) setPreferredLanguage(language i18n.Lang) {
	if language == i18n.Current() {
		return
	}
	i18n.SetLang(language)
	ebiten.SetWindowTitle(i18n.T().WindowTitle)
	g.savePreferences()
}

func (g *Game) setPreferredVolume(music bool, value float64) {
	if music {
		if audio.MusicVolume() == value {
			return
		}
		audio.SetMusicVolume(value)
	} else {
		if audio.SFXVolume() == value {
			return
		}
		audio.SetSFXVolume(value)
	}
	g.savePreferences()
}
