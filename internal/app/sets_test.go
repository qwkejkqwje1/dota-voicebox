package app

import (
	"testing"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
)

func menuIDs(items []MenuItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestSoundSets(t *testing.T) {
	a, _ := newApp(t, true)
	all := a.Menu().Sounds
	id, err := a.SaveSet(config.SoundSet{Name: "Dota 2", Emoji: "🐲", Sounds: []string{"laser", "airhorn", "nope"}, Games: []string{" Dota2.EXE "}})
	if err != nil || id != "set_dota_2" {
		t.Fatal(id, err)
	}
	if s := a.Config().Set(id); s == nil || s.Games[0] != "dota2.exe" {
		t.Fatalf("игры набора: %+v", s)
	}
	if len(a.Menu().Sounds) != len(all) {
		t.Fatal("набор ещё не включён, а меню изменилось")
	}
	a.Do("set:" + id)
	if got := menuIDs(a.Menu().Sounds); len(got) != 2 || got[0] != "laser" || got[1] != "airhorn" {
		t.Fatalf("меню набора: %v", got)
	}
	if got := menuIDs(a.PadItems()); len(got) != 2 || got[0] != "laser" {
		t.Fatalf("пульт: %v", got)
	}
	if w := a.Wheel(); w.Items[0].Act != "sound:laser" || w.Items[1].Act != "sound:airhorn" || w.Items[2].Act != "" {
		t.Fatalf("колесо: %+v", w.Items[:3])
	}
	// новый звук — и в открытый набор
	pid, err := a.AddPhrase("Мид пропал", "")
	if err != nil {
		t.Fatal(err)
	}
	waitSpeech(t, a)
	if got := menuIDs(a.Menu().Sounds); len(got) != 3 || got[2] != pid {
		t.Fatalf("новый звук не в наборе: %v", got)
	}
	// порядок и состав из окна звука
	if err := a.SetSetOrder(id, []string{"airhorn", "laser"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Config().Set(id).Sounds; got[0] != "airhorn" || got[1] != "laser" {
		t.Fatalf("порядок: %v", got)
	}
	none := []string{}
	if err := a.UpdateSound("laser", SoundEdit{Label: "Пиу", Volume: 1, Sets: &none}); err != nil {
		t.Fatal(err)
	}
	if got := menuIDs(a.Menu().Sounds); len(got) != 2 || got[0] != "airhorn" {
		t.Fatalf("убрали из набора: %v", got)
	}
	if err := a.DeleteSound("airhorn"); err != nil {
		t.Fatal(err)
	}
	for _, x := range a.Config().Set(id).Sounds {
		if x == "airhorn" {
			t.Fatal("удалённый звук остался в наборе")
		}
	}
	// по кругу: набор → все звуки
	a.Do("set:next")
	if a.Config().ActiveSet != "" || len(a.Menu().Sounds) < 5 {
		t.Fatal("set:next должен вернуть «Все звуки»")
	}
	// игра на экране включает набор, но только при смене программы
	fg := "Dota2.exe"
	a.foreground = func() string { return fg }
	a.checkForeground()
	if a.Config().ActiveSet != id || a.Status().Foreground != "dota2.exe" {
		t.Fatalf("автопереключение: %q", a.Config().ActiveSet)
	}
	a.Do("set:all")
	a.checkForeground() // та же игра — ручной выбор не перебиваем
	if a.Config().ActiveSet != "" {
		t.Fatal("набор переключился, хотя игра не менялась")
	}
	fg = "explorer.exe"
	a.checkForeground()
	fg = "cs2.exe"
	a.checkForeground()
	if r := a.RecentApps(); len(r) != 2 || r[0] != "cs2.exe" || r[1] != "dota2.exe" {
		t.Fatalf("недавние программы: %v", r)
	}
	a.UpdateConfig(func(c *config.Config) { c.AutoSet = false })
	fg = "dota2.exe"
	a.checkForeground()
	if a.Config().ActiveSet != "" {
		t.Fatal("автопереключение выключено, а набор сменился")
	}
	a.Do("set:" + id)
	if err := a.DeleteSet(id); err != nil || a.Config().ActiveSet != "" || len(a.Config().Sets) != 0 {
		t.Fatal("набор не удалён", err)
	}
	if _, err := a.SaveSet(config.SoundSet{Name: "  "}); err == nil {
		t.Fatal("набор без названия")
	}
}

func TestMenuSayAndSearch(t *testing.T) {
	a, out := newApp(t, true)
	if !a.CanSay() {
		t.Fatal("синтез речи есть")
	}
	a.MenuPick("say", "Иду на мид")
	if !out.v.Active() || !out.m.Active() {
		t.Fatal("фраза должна уйти команде (и вам)")
	}
	a.MenuPick("say", "Нужен вард")
	a.MenuPick("say", "Иду на мид")
	if p := a.RecentPhrases(); len(p) != 2 || p[0] != "Иду на мид" || p[1] != "Нужен вард" {
		t.Fatalf("недавние фразы: %v", p)
	}
	var sound, effect bool
	for _, it := range a.SearchItems() {
		sound = sound || it.Kind == "sound" && it.ID == "airhorn" && it.Label == "Дудка"
		effect = effect || it.Kind == "effect" && it.ID == "radio"
	}
	if !sound || !effect {
		t.Fatal("в поиске должны быть все звуки и эффекты")
	}
}
