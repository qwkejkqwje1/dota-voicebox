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

// AudioEndpoint — устройство звука Windows (заглушка).
type AudioEndpoint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Capture     bool   `json:"capture"`
	Default     bool   `json:"default"`
	DefaultComm bool   `json:"default_comm"`
	Rate        int    `json:"rate"`
	Bits        int    `json:"bits"`
}

func AudioEndpoints() ([]AudioEndpoint, error) { return nil, errWin }
func SetDefaultAudio(string) error             { return errWin }
func SetEndpointRate(string, int) error        { return errWin }
func OpenSoundPanel(int) error                 { return errWin }

// AppSession — программа, открывшая устройство записи (заглушка).
type AppSession struct {
	Device  string `json:"device"`
	Process string `json:"process"`
	PID     uint32 `json:"pid"`
	Active  bool   `json:"active"`
}

func CaptureSessions() ([]AppSession, error) { return nil, errWin }
