package app

import (
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Быстрое меню: модель для окна поверх игры (internal/overlay рисует, winapi показывает).

// MenuPageSize — пунктов на странице (клавиши 1–9 и 0).
const MenuPageSize = 10

// DefaultMenuEffects — эффекты на вкладке Tab, если пользователь не выбрал свои.
var DefaultMenuEffects = []string{"clean", "radio", "radio_military", "radio_broken", "telephone", "megaphone", "robot", "demon", "deep", "chipmunk"}

// MenuItem — пункт быстрого меню.
type MenuItem struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Emoji   string `json:"emoji"`
	Color   string `json:"color,omitempty"`
	Key     string `json:"key,omitempty"`    // глобальная клавиша (подсказка)
	Active  bool   `json:"active,omitempty"` // играет / включён
	Pending bool   `json:"pending,omitempty"`
	Group   string `json:"group,omitempty"` // раздел (категория звука) — для пульта с телефона
	Kind    string `json:"kind,omitempty"`  // для поиска в меню: sound | effect
	Find    string `json:"-"`               // для поиска: группа, текст фразы, id
}

// Menu — содержимое быстрого меню.
type Menu struct {
	Sounds   []MenuItem       `json:"sounds"`
	Effects  []MenuItem       `json:"effects"`
	Settings config.QuickMenu `json:"settings"`
	MenuKey  string           `json:"menu_key,omitempty"`
}

// menuSounds — звуки меню: включённый набор, выбранные пользователем или первые с панели.
func menuSounds(cfg *config.Config, lib *sounds.Library) []string {
	if set := cfg.Set(cfg.ActiveSet); set != nil {
		return setSounds(cfg, lib, set)
	}
	return userMenuSounds(cfg, lib)
}

// userMenuSounds — звуки меню без учёта набора.
func userMenuSounds(cfg *config.Config, lib *sounds.Library) []string {
	var out []string
	if len(cfg.QuickMenu.Items) > 0 {
		for _, id := range cfg.QuickMenu.Items {
			if shown(cfg, lib.Get(id)) {
				out = append(out, id)
			}
		}
		return out
	}
	out = padOrder(cfg, lib)
	if len(out) > 2*MenuPageSize {
		out = out[:2*MenuPageSize]
	}
	return out
}

func menuEffects(cfg *config.Config, presets map[string][]dsp.Spec) []string {
	list := cfg.QuickMenu.Effects
	if len(list) == 0 {
		list = DefaultMenuEffects
	}
	var out []string
	for _, id := range list {
		if _, ok := presets[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// Menu собирает быстрое меню.
func (a *App) Menu() Menu {
	a.mu.Lock()
	cfg, lib, presets, cur := a.cfg, a.lib, a.presets, a.preset
	a.mu.Unlock()
	hk := a.HotkeyMap()
	m := Menu{Settings: cfg.QuickMenu, MenuKey: hk["menu"]}
	for _, id := range menuSounds(cfg, lib) {
		s := lib.Get(id)
		m.Sounds = append(m.Sounds, MenuItem{ID: id, Label: DisplayLabel(s), Emoji: or(s.Emoji, "🔊"), Color: or(s.Color, colorFor(id)),
			Key: hk["sound:"+id], Active: a.IsPlaying(id), Pending: s.Pending && len(s.Clips) == 0})
	}
	if cfg.QuickMenu.ShowEffects {
		for _, id := range menuEffects(cfg, presets) {
			title, emoji := PresetTitle(id)
			m.Effects = append(m.Effects, MenuItem{ID: id, Label: title, Emoji: emoji, Key: hk["preset:"+id], Active: id == cur})
		}
	}
	return m
}

// PadItems — плитки панели по порядку (пульт с телефона); при включённом наборе — его плитки.
func (a *App) PadItems() []MenuItem {
	a.mu.Lock()
	cfg, lib := a.cfg, a.lib
	a.mu.Unlock()
	hk := a.HotkeyMap()
	var out []MenuItem
	for _, id := range activeOrder(cfg, lib) {
		s := lib.Get(id)
		out = append(out, MenuItem{ID: id, Label: DisplayLabel(s), Emoji: or(s.Emoji, "🔊"), Color: or(s.Color, colorFor(id)),
			Key: hk["sound:"+id], Active: a.IsPlaying(id), Pending: s.Pending && len(s.Clips) == 0, Group: s.Category})
	}
	return out
}

// EffectItems — избранные эффекты голоса (как на вкладке «Голос» быстрого меню).
func (a *App) EffectItems() []MenuItem {
	a.mu.Lock()
	cfg, presets, cur := a.cfg, a.presets, a.preset
	a.mu.Unlock()
	hk := a.HotkeyMap()
	var out []MenuItem
	for _, id := range menuEffects(cfg, presets) {
		title, emoji := PresetTitle(id)
		out = append(out, MenuItem{ID: id, Label: title, Emoji: emoji, Key: hk["preset:"+id], Active: id == cur})
	}
	return out
}

// MenuPick — выбор в быстром меню: kind = "sound" | "effect" | "say" (id — фраза для команды) |
// "read[:<голос>]" (id — текст для «Читалки») | "readme[:<голос>]" (прочитать только себе).
func (a *App) MenuPick(kind, id string) {
	if v, ok := strings.CutPrefix(kind, "readme"); ok {
		a.readPick(id, strings.TrimPrefix(v, ":"), "monitor")
		return
	}
	if v, ok := strings.CutPrefix(kind, "read"); ok {
		a.readPick(id, strings.TrimPrefix(v, ":"), "")
		return
	}
	switch kind {
	case "sound":
		if err := a.PlayOpts(id, 1); err != nil {
			a.toast(Toast{Emoji: "⚠️", Text: strings.TrimPrefix(err.Error(), "звук ")})
		}
	case "effect":
		a.SetPreset(id, true)
	case "say":
		a.rememberPhrase(id)
		if err := a.Say(id, sounds.BusBoth); err != nil {
			a.toast(Toast{Emoji: "⚠️", Text: "Не удалось озвучить", Sub: err.Error()})
			return
		}
		a.toast(Toast{Emoji: "🗣️", Text: id, Sub: "сказано команде"})
	}
}

// SearchItems — всё, что можно найти набором текста в меню поверх игры: все звуки панели и все эффекты.
func (a *App) SearchItems() []MenuItem {
	a.mu.Lock()
	cfg, lib, cur := a.cfg, a.lib, a.preset
	a.mu.Unlock()
	hk := a.HotkeyMap()
	var out []MenuItem
	for _, id := range padOrder(cfg, lib) {
		s := lib.Get(id)
		find := s.Category + " " + id
		for _, f := range s.Files {
			if t, ok := strings.CutPrefix(f, "tts:"); ok {
				find += " " + t
			}
		}
		out = append(out, MenuItem{ID: id, Label: DisplayLabel(s), Emoji: or(s.Emoji, "🔊"), Color: or(s.Color, colorFor(id)),
			Key: hk["sound:"+id], Active: a.IsPlaying(id), Pending: s.Pending && len(s.Clips) == 0, Kind: "sound", Find: find})
	}
	for _, id := range a.PresetNames() {
		title, emoji := PresetTitle(id)
		out = append(out, MenuItem{ID: id, Label: title, Emoji: emoji, Color: "#8b5cf6", Key: hk["preset:"+id], Active: id == cur,
			Kind: "effect", Find: "эффект голос effect voice " + id})
	}
	return out
}

// rememberPhrase — фраза в «недавние» (новые первыми, без повторов).
func (a *App) rememberPhrase(p string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.phrases = append([]string{p}, remove(a.phrases, p)...)
	if len(a.phrases) > 7 {
		a.phrases = a.phrases[:7]
	}
}

// RecentPhrases — недавние фразы из меню поверх игры.
func (a *App) RecentPhrases() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.phrases...)
}

// CanSay — есть ли синтез речи (озвучить набранное).
func (a *App) CanSay() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.speech != nil
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
