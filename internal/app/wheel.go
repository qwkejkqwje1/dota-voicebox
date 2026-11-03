package app

import (
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Колесо звуков: модель для окна поверх игры (internal/overlay рисует, winapi показывает).

// WheelItem — сектор колеса.
type WheelItem struct {
	Act     string `json:"act"` // sound:<id> | preset:<id> | stop | mic_mute; "" — пусто
	Label   string `json:"label,omitempty"`
	Emoji   string `json:"emoji,omitempty"`
	Color   string `json:"color,omitempty"`
	Active  bool   `json:"active,omitempty"`  // звук играет / эффект включён
	Pending bool   `json:"pending,omitempty"` // фраза ещё готовится
	Missing bool   `json:"missing,omitempty"` // звук удалён или спрятан — сектор пустой
}

// Wheel — колесо: 8 секторов по часовой стрелке от верхнего.
type Wheel struct {
	Items    []WheelItem  `json:"items"`
	Auto     bool         `json:"auto"` // первые 8 звуков с панели
	Settings config.Wheel `json:"settings"`
	Key      string       `json:"key,omitempty"`
}

// wheelActs — действия секторов: свои или первые 8 звуков с панели (включённого набора).
func wheelActs(cfg *config.Config, lib *sounds.Library) []string {
	out := make([]string, config.WheelSlots)
	if len(cfg.Wheel.Items) == 0 {
		for i, id := range activeOrder(cfg, lib) {
			if i == config.WheelSlots {
				break
			}
			out[i] = "sound:" + id
		}
		return out
	}
	copy(out, cfg.Wheel.Items)
	return out
}

// Wheel собирает колесо.
func (a *App) Wheel() Wheel {
	a.mu.Lock()
	cfg, lib, presets, cur := a.cfg, a.lib, a.presets, a.preset
	a.mu.Unlock()
	w := Wheel{Auto: len(cfg.Wheel.Items) == 0, Settings: cfg.Wheel, Key: a.HotkeyMap()["wheel"]}
	for _, act := range wheelActs(cfg, lib) {
		it := WheelItem{Act: act}
		switch {
		case act == "":
		case strings.HasPrefix(act, "sound:"):
			id := strings.TrimPrefix(act, "sound:")
			s := lib.Get(id)
			if !shown(cfg, s) {
				it.Missing, it.Label = true, id
				break
			}
			it.Label, it.Emoji, it.Color = DisplayLabel(s), or(s.Emoji, "🔊"), or(s.Color, colorFor(id))
			it.Active, it.Pending = a.IsPlaying(id), s.Pending && len(s.Clips) == 0
		case strings.HasPrefix(act, "preset:"):
			id := strings.TrimPrefix(act, "preset:")
			if _, ok := presets[id]; !ok {
				it.Missing, it.Label = true, id
				break
			}
			it.Label, it.Emoji = PresetTitle(id)
			it.Color, it.Active = "#8b5cf6", id == cur
		case act == "stop":
			it.Label, it.Emoji, it.Color = "Стоп", "⏹️", "#ef4444"
		case act == "mic_mute":
			it.Label, it.Emoji, it.Color = "Выключить микрофон", "🎙️", "#22c55e"
			if cfg.Mic.Mute {
				it.Label, it.Emoji, it.Color, it.Active = "Включить микрофон", "🔇", "#ef4444", true
			}
		default:
			it.Missing, it.Label = true, act
		}
		w.Items = append(w.Items, it)
	}
	return w
}

// WheelPick — выбрали сектор колеса.
func (a *App) WheelPick(act string) {
	switch {
	case act == "":
	case strings.HasPrefix(act, "sound:"):
		a.MenuPick("sound", strings.TrimPrefix(act, "sound:"))
	case strings.HasPrefix(act, "preset:"):
		a.SetPreset(strings.TrimPrefix(act, "preset:"), true)
	case act == "stop", act == "mic_mute":
		a.Do(act)
	}
}

// SetWheel сохраняет настройки колеса (включение меняет горячие клавиши — тогда полная перезагрузка).
func (a *App) SetWheel(w config.Wheel) error {
	if a.Config().Wheel.Enabled != w.Enabled {
		return a.UpdateConfig(func(c *config.Config) { c.Wheel = w })
	}
	return a.saveQuiet(func(c *config.Config) { c.Wheel = w })
}

// SetRemote сохраняет настройки пульта с телефона (сервер пульта перезапускается по OnChange).
// Ключ, если его ещё нет, создаётся при включении (config.Fix).
func (a *App) SetRemote(r config.Remote) error {
	return a.saveQuiet(func(c *config.Config) { c.Remote = r })
}

// SaveRemoteKey — новый ключ пульта: старые ссылки и QR-коды перестают работать.
func (a *App) SaveRemoteKey(key string) error {
	return a.saveQuiet(func(c *config.Config) { c.Remote.Key = key })
}
