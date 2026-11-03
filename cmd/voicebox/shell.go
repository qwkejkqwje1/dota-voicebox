package main

import (
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/overlay"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// menuSource — быстрое меню поверх игры берёт данные у приложения.
type menuSource struct{ a *app.App }

func (m menuSource) MenuData() overlay.Data {
	mm := m.a.Menu()
	s := mm.Settings
	d := overlay.Data{Layout: s.Layout, Position: s.Position, X: s.X, Y: s.Y, Monitor: s.Monitor, Opacity: s.Opacity,
		Scale: s.Scale, TimeoutSec: s.TimeoutSec, CloseAfterPick: s.CloseAfterPick}
	conv := func(in []app.MenuItem) []overlay.Item {
		out := make([]overlay.Item, 0, len(in))
		for _, it := range in {
			out = append(out, overlay.Item{ID: it.ID, Emoji: it.Emoji, Label: it.Label, Color: it.Color,
				Hint: keys.Display(it.Key), Active: it.Active, Pending: it.Pending, Kind: it.Kind, Find: it.Find})
		}
		return out
	}
	d.Sounds = conv(mm.Sounds)
	d.Effects = conv(mm.Effects)
	d.Search = conv(m.a.SearchItems())
	for i := range d.Search {
		if d.Search[i].Kind == "effect" && d.Search[i].Hint == "" {
			d.Search[i].Hint = "эффект"
		}
	}
	d.Phrases = m.a.RecentPhrases()
	d.CanSay = m.a.CanSay()
	if cfg := m.a.Config(); len(d.Sounds) == 0 && cfg.Set(cfg.ActiveSet) != nil {
		title, _ := app.SetTitle(cfg, cfg.ActiveSet)
		d.Empty = "В наборе «" + title + "» нет звуков · Enter — поиск"
	}
	return d
}

func (m menuSource) Pick(kind, id string) { m.a.MenuPick(kind, id) }
func (m menuSource) StopAll()             { m.a.StopAll() }

// wheelSource — колесо звуков берёт данные у приложения.
type wheelSource struct{ a *app.App }

func (w wheelSource) WheelData() overlay.WheelData {
	ww := w.a.Wheel()
	d := overlay.WheelData{Center: ww.Settings.Center, Scale: ww.Settings.Scale, Opacity: ww.Settings.Opacity}
	for _, it := range ww.Items {
		if it.Missing {
			it = app.WheelItem{}
		}
		d.Items = append(d.Items, overlay.WheelItem{Act: it.Act, Emoji: it.Emoji, Label: it.Label, Color: it.Color,
			Active: it.Active, Pending: it.Pending})
	}
	return d
}

func (w wheelSource) WheelPick(act string) { w.a.WheelPick(act) }

// trayItems — меню значка в трее.
func trayItems(a *app.App) []winapi.TrayItem {
	cfg := a.Config()
	hk := a.HotkeyMap()
	cur := a.Preset()
	withKey := func(title, action string) string {
		if k := keys.Display(hk[action]); k != "" {
			return title + "\t" + k
		}
		return title
	}
	var fx []winapi.TrayItem
	for _, id := range a.PresetNames() {
		title, _ := app.PresetTitle(id)
		fx = append(fx, winapi.TrayItem{ID: "preset:" + id, Title: title, Checked: id == cur})
	}
	items := []winapi.TrayItem{
		{ID: "open", Title: withKey("Открыть VoiceBox", "ui"), Default: true},
		{ID: "menu", Title: withKey("Быстрое меню", "menu"), Disabled: !cfg.QuickMenu.Enabled},
		{ID: "wheel", Title: withKey("Колесо звуков", "wheel"), Disabled: !cfg.Wheel.Enabled},
		{Sep: true},
		{Title: "Эффект голоса", Sub: fx, Disabled: len(fx) == 0},
		{ID: "mic_mute", Title: withKey("Микрофон выключен", "mic_mute"), Checked: cfg.Mic.Mute},
		{ID: "stop", Title: withKey("Остановить звуки", "stop")},
		{Sep: true},
		{ID: "dota", Title: "Модуль Dota 2", Checked: cfg.Dota.Enabled},
		{Sep: true},
		{ID: "quit", Title: "Выход"},
	}
	return items
}

func trayAction(a *app.App, sh *winapi.Shell, id string, show, quit func()) {
	switch id {
	case "open":
		show()
	case "menu":
		sh.OpenMenu()
	case "wheel":
		sh.OpenWheel(0)
	case "mic_mute", "stop":
		a.Do(id)
	case "dota":
		a.UpdateConfig(func(c *config.Config) { c.Dota.Enabled = !c.Dota.Enabled })
	case "quit":
		quit()
	default:
		if p, ok := strings.CutPrefix(id, "preset:"); ok {
			a.SetPreset(p, true)
		}
	}
}

// trayTip — подсказка значка: эффект и состояние микрофона.
func trayTip(a *app.App) string {
	st := a.Status()
	title, _ := app.PresetTitle(st.Preset)
	tip := "VoiceBox — " + title
	if st.MicMute {
		tip += " · микрофон выключен"
	}
	return tip
}
