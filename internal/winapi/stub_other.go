//go:build !windows

package winapi

import (
	"errors"
	"log"

	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Заглушки для сборки и тестов вне Windows.

type Hotkeys struct{}

func NewHotkeys(func(string)) *Hotkeys { return &Hotkeys{} }
func (h *Hotkeys) Set(c []keys.Combo) []error {
	if len(c) > 0 {
		log.Printf("горячие клавиши доступны только в Windows (%d шт. пропущено)", len(c))
	}
	return nil
}

var errWin = errors.New("только Windows")

func PressKey(uint32, bool) error       { return errWin }
func IsKeyDown(uint32) bool             { return false }
func NewTTS(string, int) sounds.TTSFunc { return nil }
func SteamRoot() string                 { return "" }
func SteamRunning() bool                { return false }
func DotaRunning() bool                 { return false }
func ShutdownSteam(string) error        { return errWin }
func StartSteam(string) error           { return errWin }
func RunElevated(string, string) error  { return errWin }
func OpenURL(string) error              { return errWin }
func Autostart() bool                   { return false }
func SetAutostart(bool) error           { return errWin }
func Alert(title, text string)          { log.Printf("%s: %s", title, text) }
func SetHighPriority()                  {}

func StartKeyHook(chan<- combo.Event) error { return errWin }
