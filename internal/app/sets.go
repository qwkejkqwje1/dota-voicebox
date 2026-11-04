package app

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// Наборы звуков: свои плитки для каждой игры. Включённый набор — это панель, быстрое меню, колесо
// (если в нём не выбраны свои секторы) и пульт с телефона. Набор включается сам, когда его игра на экране.

// Game — известная игра (для выбора в окне набора).
type Game struct {
	Exe   []string `json:"exe"` // программы игры (в нижнем регистре)
	Name  string   `json:"name"`
	Emoji string   `json:"emoji"`
}

// KnownGames — популярные игры с голосовым чатом.
var KnownGames = []Game{
	{Exe: []string{"dota2.exe"}, Name: "Dota 2", Emoji: "🐲"},
	{Exe: []string{"cs2.exe"}, Name: "Counter-Strike 2", Emoji: "🔫"},
	{Exe: []string{"valorant-win64-shipping.exe"}, Name: "Valorant", Emoji: "🎯"},
	{Exe: []string{"league of legends.exe"}, Name: "League of Legends", Emoji: "⚔️"},
	{Exe: []string{"deadlock.exe"}, Name: "Deadlock", Emoji: "🔒"},
	{Exe: []string{"fortniteclient-win64-shipping.exe"}, Name: "Fortnite", Emoji: "🏗️"},
	{Exe: []string{"r5apex.exe", "r5apex_dx12.exe"}, Name: "Apex Legends", Emoji: "🦾"},
	{Exe: []string{"tslgame.exe"}, Name: "PUBG", Emoji: "🍳"},
	{Exe: []string{"overwatch.exe"}, Name: "Overwatch 2", Emoji: "🛡️"},
	{Exe: []string{"rustclient.exe"}, Name: "Rust", Emoji: "🪓"},
	{Exe: []string{"gta5.exe", "gta5_enhanced.exe"}, Name: "GTA V", Emoji: "🚗"},
	{Exe: []string{"robloxplayerbeta.exe"}, Name: "Roblox", Emoji: "🧱"},
}

// setSounds — плитки набора по порядку (удалённые и спрятанные пропускаются).
func setSounds(cfg *config.Config, lib *sounds.Library, set *config.SoundSet) []string {
	out := []string{}
	for _, id := range set.Sounds {
		if shown(cfg, lib.Get(id)) {
			out = append(out, id)
		}
	}
	return out
}

// activeOrder — плитки включённого набора или все плитки панели.
func activeOrder(cfg *config.Config, lib *sounds.Library) []string {
	if set := cfg.Set(cfg.ActiveSet); set != nil {
		return setSounds(cfg, lib, set)
	}
	return padOrder(cfg, lib)
}

// SetTitle — название набора со значком ("" — все звуки).
func SetTitle(cfg *config.Config, id string) (title, emoji string) {
	if set := cfg.Set(id); set != nil {
		return set.Name, or(set.Emoji, "🗂️")
	}
	return i18n.T("Все звуки"), "🎛️"
}

// SetActiveSet включает набор ("" — все звуки).
func (a *App) SetActiveSet(id string, announce bool) error {
	cfg := a.Config()
	if id != "" && cfg.Set(id) == nil {
		return fmt.Errorf("нет набора %q", id)
	}
	if cfg.ActiveSet == id {
		return nil
	}
	if err := a.saveQuiet(func(c *config.Config) { c.ActiveSet = id }); err != nil {
		return err
	}
	title, emoji := SetTitle(cfg, id)
	log.Printf("Набор звуков: %s", title)
	if announce {
		a.toast(Toast{Emoji: emoji, Text: "Набор «" + title + "»", Sub: "звуки на панели, в меню и на колесе"})
	}
	return nil
}

// cycleSet — следующий/предыдущий набор по кругу (с «Все звуки»).
func (a *App) cycleSet(forward bool) {
	cfg := a.Config()
	ids := []string{""}
	cur := 0
	for i, s := range cfg.Sets {
		ids = append(ids, s.ID)
		if s.ID == cfg.ActiveSet {
			cur = i + 1
		}
	}
	if len(ids) == 1 {
		a.toast(Toast{Emoji: "🗂️", Text: "Наборов пока нет", Sub: "создайте их на вкладке «Звуки»"})
		return
	}
	d := 1
	if !forward {
		d = len(ids) - 1
	}
	a.SetActiveSet(ids[(cur+d)%len(ids)], true)
}

// SaveSet создаёт или изменяет набор; возвращает его id.
func (a *App) SaveSet(s config.SoundSet) (string, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return "", fmt.Errorf("назовите набор")
	}
	if len([]rune(s.Name)) > 40 {
		s.Name = string([]rune(s.Name)[:40])
	}
	cfg := a.Config()
	if s.ID == "" || cfg.Set(s.ID) == nil {
		base := "set_" + Slug(s.Name)
		s.ID = base
		for i := 2; cfg.Set(s.ID) != nil; i++ {
			s.ID = fmt.Sprintf("%s_%d", base, i)
		}
	}
	if s.Sounds == nil {
		s.Sounds = []string{}
	}
	err := a.saveQuiet(func(c *config.Config) {
		if old := c.Set(s.ID); old != nil {
			*old = s
		} else {
			c.Sets = append(c.Sets, s)
		}
	})
	return s.ID, err
}

// DeleteSet удаляет набор (звуки остаются).
func (a *App) DeleteSet(id string) error {
	return a.saveQuiet(func(c *config.Config) {
		out := c.Sets[:0:0]
		for _, s := range c.Sets {
			if s.ID != id {
				out = append(out, s)
			}
		}
		c.Sets = out
		if c.ActiveSet == id {
			c.ActiveSet = ""
		}
	})
}

// SetSetOrder — новый порядок плиток набора (перетаскивание на панели).
func (a *App) SetSetOrder(id string, order []string) error {
	return a.saveQuiet(func(c *config.Config) {
		if s := c.Set(id); s != nil {
			s.Sounds = mergePad(order, s.Sounds)
		}
	})
}

// setMembership — звук входит в наборы ids и только в них.
func setMembership(c *config.Config, sound string, ids []string) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for i := range c.Sets {
		s := &c.Sets[i]
		has := false
		for _, x := range s.Sounds {
			has = has || x == sound
		}
		switch {
		case want[s.ID] && !has:
			s.Sounds = append(s.Sounds, sound)
		case !want[s.ID] && has:
			s.Sounds = remove(s.Sounds, sound)
		}
	}
}

// ---------- игра на переднем плане ----------

// не игры: рабочий стол, меню «Пуск», диспетчер задач…
var notGames = map[string]bool{"explorer.exe": true, "searchhost.exe": true, "searchapp.exe": true, "startmenuexperiencehost.exe": true,
	"shellexperiencehost.exe": true, "applicationframehost.exe": true, "taskmgr.exe": true, "lockapp.exe": true,
	"textinputhost.exe": true, "voicebox.exe": true, "steamwebhelper.exe": true, "msedgewebview2.exe": true}

// ForegroundLoop раз в 2 секунды смотрит, какая программа на экране: запоминает её для выбора в окне набора
// и включает набор этой игры (если включено «автопереключение»).
func (a *App) ForegroundLoop() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for range t.C {
		a.checkForeground()
	}
}

func (a *App) checkForeground() {
	fg := a.foreground
	if fg == nil {
		fg = winapi.ForegroundApp
	}
	exe := strings.ToLower(strings.TrimSpace(fg()))
	if exe == "" {
		return
	}
	a.fgMu.Lock()
	changed := exe != a.fgLast
	a.fgLast = exe
	if changed && !notGames[exe] {
		a.fgRecent = append([]string{exe}, remove(a.fgRecent, exe)...)
		if len(a.fgRecent) > 12 {
			a.fgRecent = a.fgRecent[:12]
		}
	}
	a.fgMu.Unlock()
	if !changed {
		return
	}
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	if !cfg.AutoSet {
		return
	}
	for _, s := range cfg.Sets {
		for _, g := range s.Games {
			if g == exe {
				if cfg.ActiveSet != s.ID {
					a.SetActiveSet(s.ID, true)
				}
				return
			}
		}
	}
}

// RecentApps — программы, которые недавно были на экране (новые первыми).
func (a *App) RecentApps() []string {
	a.fgMu.Lock()
	defer a.fgMu.Unlock()
	return append([]string{}, a.fgRecent...)
}
