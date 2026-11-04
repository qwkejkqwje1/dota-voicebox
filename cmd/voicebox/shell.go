package main

import (
	"fmt"
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
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
			d.Search[i].Hint = i18n.T("эффект")
		}
	}
	d.Phrases = m.a.RecentPhrases()
	d.CanSay = m.a.CanSay()
	voices, cur, hist := m.a.ReaderMenu()
	d.Voices, d.Voice, d.ReadHist = conv(voices), cur, hist
	if cfg := m.a.Config(); len(d.Sounds) == 0 && cfg.Set(cfg.ActiveSet) != nil {
		title, _ := app.SetTitle(cfg, cfg.ActiveSet)
		d.Empty = fmt.Sprintf(i18n.T("В наборе «%s» нет звуков · Enter — поиск"), title)
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
	// микрофоны: настоящие устройства, отмечен выбранный (или «как в Windows»)
	mics := []winapi.TrayItem{{ID: "mic:", Title: i18n.T("Как в Windows (по умолчанию)"), Checked: cfg.Devices.Mic == ""}}
	if capture, _, err := audio.Devices(); err == nil {
		for _, d := range capture {
			if !d.Virtual && !audio.IsCableInput(d.Name) {
				mics = append(mics, winapi.TrayItem{ID: "mic:" + d.Name, Title: d.Name, Checked: strings.EqualFold(cfg.Devices.Mic, d.Name)})
			}
		}
	}
	if k := keys.Display(hk["mic:next"]); k != "" {
		mics = append(mics, winapi.TrayItem{Sep: true}, winapi.TrayItem{ID: "mic:next", Title: i18n.T("Следующий микрофон") + "\t" + k})
	}
	items := []winapi.TrayItem{
		{ID: "open", Title: withKey(i18n.T("Открыть VoiceBox"), "ui"), Default: true},
		{ID: "menu", Title: withKey(i18n.T("Быстрое меню"), "menu"), Disabled: !cfg.QuickMenu.Enabled},
		{ID: "wheel", Title: withKey(i18n.T("Колесо звуков"), "wheel"), Disabled: !cfg.Wheel.Enabled},
		{ID: "reader", Title: withKey(i18n.T("Читалка"), "reader")},
		{Sep: true},
		{Title: i18n.T("Эффект голоса"), Sub: fx, Disabled: len(fx) == 0},
		{Title: i18n.T("Микрофон"), Sub: mics},
		{ID: "mic_mute", Title: withKey(i18n.T("Микрофон выключен"), "mic_mute"), Checked: cfg.Mic.Mute},
		{ID: "stop", Title: withKey(i18n.T("Остановить звуки"), "stop")},
		{Sep: true},
		{ID: "dota", Title: i18n.T("Модуль Dota 2"), Checked: cfg.Dota.Enabled},
		{ID: "chat_read", Title: withKey(i18n.T("Читать чат Dota 2 вслух"), "chat_read"), Checked: cfg.Reader.Chat.Enabled},
		{Sep: true},
		{ID: "quit", Title: i18n.T("Выход")},
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
	case "reader":
		sh.OpenReader()
	case "mic_mute", "stop", "mic:next", "chat_read":
		a.Do(id)
	case "dota":
		a.UpdateConfig(func(c *config.Config) { c.Dota.Enabled = !c.Dota.Enabled })
	case "quit":
		quit()
	default:
		if p, ok := strings.CutPrefix(id, "preset:"); ok {
			a.SetPreset(p, true)
		}
		if m, ok := strings.CutPrefix(id, "mic:"); ok {
			a.UpdateConfig(func(c *config.Config) { c.Devices.Mic = m })
		}
	}
}

// trayTip — подсказка значка: эффект и состояние микрофона.
func trayTip(a *app.App) string {
	st := a.Status()
	title, _ := app.PresetTitle(st.Preset)
	tip := "VoiceBox — " + title
	if st.MicMute {
		tip += " · " + i18n.T("микрофон выключен")
	}
	return tip
}
