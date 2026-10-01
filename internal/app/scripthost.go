package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/script"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// scriptHost — то, что приложение даёт скриптам.
type scriptHost struct{ a *App }

func busFor(to string) (string, error) {
	switch strings.ToLower(to) {
	case "":
		return "", nil
	case "team", "voice", "войс":
		return "voice", nil
	case "me", "monitor", "я":
		return "monitor", nil
	case "both", "all":
		return "both", nil
	}
	return "", fmt.Errorf("to: %q — нужно team, me или both", to)
}

func (h *scriptHost) Play(ref, to string, volume float64) error {
	bus, err := busFor(to)
	if err != nil {
		return err
	}
	if bus != "" {
		ref, _, _ = strings.Cut(ref, "@")
		ref += "@" + bus
	}
	if volume <= 0 {
		volume = 1
	}
	return h.a.PlayOpts(ref, volume)
}

// Say синтезирует речь (Windows TTS, с кэшем) и играет её. Не блокирует скрипт.
func (h *scriptHost) Say(text, to string) error {
	bus, err := busFor(to)
	if err != nil {
		return err
	}
	if bus == "" {
		bus = "both"
	}
	h.a.mu.Lock()
	tts := h.a.tts
	h.a.mu.Unlock()
	if tts == nil {
		return fmt.Errorf("синтез речи недоступен на этой системе")
	}
	go func() {
		clip, err := tts(text)
		if err != nil {
			h.a.logf("say: %v", err)
			return
		}
		h.a.playClip("say", clip, 1, "", sounds.ParseBus(bus))
	}()
	return nil
}

func (h *scriptHost) Stop(ref string) {
	h.a.out.Voice().Stop(ref)
	h.a.out.Monitor().Stop(ref)
}

func (h *scriptHost) SetPreset(name string) error {
	h.a.mu.Lock()
	_, ok := h.a.presets[name]
	h.a.mu.Unlock()
	if !ok {
		return fmt.Errorf("нет пресета %q", name)
	}
	h.a.SetPreset(name, true)
	return nil
}

func (h *scriptHost) Preset() string    { return h.a.Preset() }
func (h *scriptHost) Action(act string) { go h.a.Do(act) }
func (h *scriptHost) Game() script.Game { return h.a.Game() }

// Game — снимок состояния игры для скриптов.
func (a *App) Game() script.Game {
	g := script.Game{Clock: a.clock.Now()}
	a.gsiMu.Lock()
	defer a.gsiMu.Unlock()
	g.Connected = !a.lastGSI.IsZero() && time.Since(a.lastGSI) < 40*time.Second
	g.Raw = a.rawState
	if st := a.prevState; st != nil && st.Map != nil {
		g.State = strings.TrimPrefix(st.Map.GameState, "DOTA_GAMERULES_STATE_")
		g.Paused, g.Daytime = st.Map.Paused, st.Map.Daytime
		if st.Hero != nil {
			g.Hero = strings.TrimPrefix(st.Hero.Name, "npc_dota_hero_")
		}
	}
	return g
}
