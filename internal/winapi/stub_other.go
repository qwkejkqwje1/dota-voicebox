//go:build !windows

package winapi

import (
	"errors"
	"log"
	"os/exec"

	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/overlay"
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

func PressKey(uint32, bool) error      { return errWin }
func IsKeyDown(uint32) bool            { return false }
func SteamRoot() string                { return "" }
func SteamRunning() bool               { return false }
func DotaRunning() bool                { return false }
func ShutdownSteam(string) error       { return errWin }
func StartSteam(string) error          { return errWin }
func RunElevated(string, string) error { return errWin }
func AllowFirewall(string, int) error  { return errWin }
func OpenFirewallApps() error          { return errWin }
func OpenNetworkSettings() error       { return errWin }
func OpenURL(string) error             { return errWin }
func Autostart() bool                  { return false }
func SetAutostart(bool) error          { return errWin }
func HasShortcut(string) bool          { return false }
func SetShortcut(string, bool) error   { return errWin }
func HealShortcuts() []string          { return nil }
func Alert(title, text string)         { log.Printf("%s: %s", title, text) }
func SetHighPriority()                 {}
func AllowForeground()                 {}

func StartKeyHook(chan<- combo.Event) error { return errWin }

func SetHookFilter(func(uint32, uint32, bool) bool, func(uintptr, int32, int32, uint32) bool) {}

// Shell — значок в трее и меню поверх игры (только Windows).
type Shell struct{}

func StartShell(ShellOptions) (*Shell, error) { return nil, errWin }
func (s *Shell) ToggleMenu()                  {}
func (s *Shell) OpenMenu()                    {}
func (s *Shell) CloseMenu()                   {}
func (s *Shell) RefreshMenu()                 {}
func (s *Shell) MenuVisible() bool            { return false }
func (s *Shell) OpenWheel(uint32)             {}
func (s *Shell) OpenSay()                     {}
func (s *Shell) OpenReader()                  {}
func (s *Shell) Toast(overlay.ToastView)      {}
func (s *Shell) SetTooltip(string)            {}
func (s *Shell) Balloon(string, string)       {}
func (s *Shell) Close()                       {}

// NewTTS — вне Windows синтеза речи нет.
func NewTTS(cacheDir, voice string, rate int) *TTS { return nil }

func (t *TTS) synth([]ttsItem) error    { return errWin }
func CloseTTS()                         {}
func hideWindow(*exec.Cmd)              {}
func (t *TTS) Voices() ([]Voice, error) { return nil, errWin }

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

// ForegroundApp — программа активного окна (только Windows).
func ForegroundApp() string { return "" }
