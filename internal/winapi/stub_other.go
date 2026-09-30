//go:build !windows

package winapi

import (
	"errors"
	"log"

	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Заглушки для сборки и тестов вне Windows.

type Hotkeys struct{}

func NewHotkeys(func(string)) *Hotkeys { return &Hotkeys{} }
func (h *Hotkeys) Set(c []keys.Combo) []error {
	log.Printf("горячие клавиши доступны только в Windows (%d шт. пропущено)", len(c))
	return nil
}
func PressKey(uint32, bool) error       { return errors.New("только Windows") }
func IsKeyDown(uint32) bool             { return false }
func NewTTS(string, int) sounds.TTSFunc { return nil }
func SteamRoot() string                 { return "" }
