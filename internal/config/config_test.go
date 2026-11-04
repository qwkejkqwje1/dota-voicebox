package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

func TestDefaultAndOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if created, err := EnsureFile(p, "ru"); err != nil || !created {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || c.Hotkeys["Alt+TILDE"] != "menu" || c.Dota.Enabled || len(c.Dota.Reminders) != 7 {
		t.Fatalf("%+v", c)
	}
	if !c.Dota.Reminders[0].Enabled || c.Dota.Reminders[0].Every != 420 || c.Dota.VoiceTo != "monitor" {
		t.Fatalf("руна мудрости: %+v", c.Dota.Reminders[0])
	}
	if !c.QuickMenu.Enabled || c.QuickMenu.Scale != 1 || len(c.Pad) < 10 || c.Scripts["gank_mid"] {
		t.Fatalf("%+v %v", c.QuickMenu, c.Pad)
	}
	for _, id := range c.Pad {
		d, ok := c.Sounds[id]
		if !ok || d.Emoji == "" || d.Label == "" {
			t.Fatalf("плитка %s: %+v", id, d)
		}
	}
	for _, f := range c.Sounds["dota_gank"].Files {
		if b, ok := strings_cut(f, "builtin:"); ok {
			if _, ok := sounds.Builtin(b); !ok {
				t.Fatalf("нет встроенного %s", b)
			}
		}
	}
	// свои клавиши заменяют клавиши по умолчанию; при переходе на v5 добавляется только клавиша «Читалки»
	os.WriteFile(p, []byte(`{"version":3,"ptt":{"key":"B","auto":true},"hotkeys":{"F9":"stop"}}`), 0o644)
	c, err = Load(p)
	if err != nil || c.PTT.Key != "B" || len(c.Hotkeys) != 2 || c.Hotkeys[ReaderKey] != "reader" || len(c.Dota.Reminders) != 7 {
		t.Fatalf("%v %+v", err, c.Hotkeys)
	}
	os.WriteFile(p, []byte(`{"version":5,"hotkeys":{"F9":"stop"}}`), 0o644)
	if c, err = Load(p); err != nil || len(c.Hotkeys) != 1 {
		t.Fatalf("%v %+v", err, c.Hotkeys)
	}
	// клавиша занята или у читалки уже есть своя — не трогаем
	os.WriteFile(p, []byte(`{"version":4,"hotkeys":{"Ctrl+Alt+R":"stop","F8":"menu"}}`), 0o644)
	if c, err = Load(p); err != nil || len(c.Hotkeys) != 2 || c.Hotkeys["Ctrl+Alt+R"] != "stop" {
		t.Fatalf("%v %+v", err, c.Hotkeys)
	}
	os.WriteFile(p, []byte(`{"version":4,"hotkeys":{"F7":"reader"}}`), 0o644)
	if c, err = Load(p); err != nil || len(c.Hotkeys) != 1 {
		t.Fatalf("%v %+v", err, c.Hotkeys)
	}
}

func strings_cut(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return "", false
}

const v2Config = `{
  "version": 2,
  "mic_gain": 1.3,
  "sounds": {
    "siren":       { "label": "🚨 Мид ганкают", "files": ["builtin:siren"], "bus": "both", "cooldown": 3, "mode": "interrupt" },
    "wisdom_rune": { "files": ["builtin:wisdom_rune"], "bus": "monitor", "mode": "ignore" },
    "power_rune":  { "files": ["builtin:power_rune"], "bus": "monitor", "mode": "ignore" },
    "my_laugh":    { "label": "Смех", "files": ["sounds/laugh.mp3"] }
  },
  "hotkeys": { "Num2": "sound:wisdom_rune@both", "NumDot": "stop", "Ctrl+Alt+V": "ui", "Num1": "preset:robot" },
  "timers": [
    { "id": "wisdom_rune", "label": "Руна мудрости", "sound": "wisdom_rune", "start": 420, "every": 420, "warn_before": 20 },
    { "id": "power_rune",  "label": "Руна силы", "sound": "power_rune", "start": 360, "every": 120, "skip": [360], "warn_before": 10, "disabled": true },
    { "id": "custom", "label": "Мой таймер", "sound": "my_laugh@monitor", "at": [600], "warn_before": 5 }
  ],
  "scripts": { "gank_mid": true },
  "ui": { "port": 3001, "keep_running_on_close": false, "open_on_start": true }
}`

func TestMigrateV2(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(v2Config), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || c.MicGain != 1.3 || len(c.Timers) != 0 || !c.Dota.Enabled || !c.UI.KeepRunningOnClose {
		t.Fatalf("%+v", c)
	}
	rs := c.Dota.Reminders
	if len(rs) != 4 || rs[0].ID != "wisdom_rune" || !rs[0].Enabled || rs[0].Before != 20 || rs[1].Enabled || rs[1].Skip[0] != 360 || rs[3].ID != "support_runes" {
		t.Fatalf("напоминания: %+v", rs)
	}
	if rs[2].Sound != "my_laugh" || rs[0].Sound != "" {
		t.Fatalf("сигналы напоминаний: %+v", rs)
	}
	s := c.Sounds["siren"]
	if s.Emoji != "🚨" || s.Label != "Мид ганкают" || !s.Dota || s.Hidden {
		t.Fatalf("siren: %+v", s)
	}
	if w := c.Sounds["wisdom_rune"]; !w.Hidden || !w.Dota {
		t.Fatalf("служебный звук должен спрятаться: %+v", w)
	}
	if c.Scripts["gank_mid"] {
		t.Fatal("скрипт gank_mid должен выключиться (заменён кнопкой)")
	}
	if _, ok := c.Sounds["dota_gank"]; !ok {
		t.Fatal("нет кнопки ганка")
	}
	// Num1 был занят пользователем — не трогаем; Ctrl+Num1 свободен
	if c.Hotkeys["Num1"] != "preset:robot" || c.Hotkeys["Ctrl+Num1"] != "sound:dota_mia" {
		t.Fatalf("клавиши: %v", c.Hotkeys)
	}
	if c.Hotkeys["Alt+TILDE"] != "menu" || c.Hotkeys["NumDot"] != "stop" || c.Hotkeys["Pause"] != "" {
		t.Fatalf("клавиши: %v", c.Hotkeys)
	}
	// пользовательские звуки — первыми на панели, затем новые
	if len(c.Pad) < 5 || c.Pad[0] != "siren" || c.Pad[1] != "my_laugh" {
		t.Fatalf("панель: %v", c.Pad)
	}
	for _, id := range c.Pad {
		if c.Sounds[id].Hidden {
			t.Fatalf("скрытый звук на панели: %s", id)
		}
	}
	// повторная загрузка сохранённого — без повторной миграции
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(p)
	if err != nil || len(c2.Dota.Reminders) != 4 || len(c2.Pad) != len(c.Pad) || c2.Hotkeys["Num1"] != "preset:robot" {
		t.Fatalf("%v %+v", err, c2.Dota.Reminders)
	}
}

func TestMigrateV1AndBackups(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, []byte(`{"mic_gain": 1.7, "hotkeys": {"F9": "stop"}}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || c.MicGain != 1.7 || c.Hotkeys["F9"] != "stop" || !c.Update.AutoCheck {
		t.Fatalf("%+v", c)
	}
	// в v1 скрипт gank_mid был включён по умолчанию → кнопки ганка с клавишами
	if c.Hotkeys["Num1"] != "sound:dota_gank" || c.Hotkeys["Alt+TILDE"] != "menu" {
		t.Fatalf("%v", c.Hotkeys)
	}
	for i := 0; i < 8; i++ {
		c.MicGain = float64(i)
		if err := Save(p, c); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(Backups(p)); n != 5 {
		t.Fatalf("копий: %d", n)
	}
	if err := Restore(p, Backups(p)[0]); err != nil {
		t.Fatal(err)
	}
	c2, _ := Load(p)
	if c2.MicGain != 6 {
		t.Fatalf("restore: %v", c2.MicGain)
	}
}

func TestFixAndHelpers(t *testing.T) {
	c := &Config{}
	c.Dota.Reminders = []Reminder{{ID: "a"}, {ID: "a"}, {}}
	c.Pad = []string{"x", "y", "x", ""}
	c.QuickMenu.Position = "nowhere"
	c.Fix()
	if c.Dota.Reminders[1].ID == "a" || c.Dota.Reminders[2].ID == "" || c.Dota.Reminders[1].ID == c.Dota.Reminders[2].ID {
		t.Fatalf("%+v", c.Dota.Reminders)
	}
	if len(c.Pad) != 2 || c.QuickMenu.Position != "center" || c.QuickMenu.Scale != 1 || c.Dota.VoiceTo != "monitor" {
		t.Fatalf("%+v", c)
	}
	if e, l := SplitEmoji("🚨 Мид ганкают"); e != "🚨" || l != "Мид ганкают" {
		t.Fatal(e, l)
	}
	if e, l := SplitEmoji("Мид 2"); e != "" || l != "Мид 2" {
		t.Fatal(e, l)
	}
	if FreeKey(map[string]string{"ctrl+num1": "x"}, "Ctrl+Num1") {
		t.Fatal("регистр не должен влиять")
	}
}

func TestFixQuickMenuLook(t *testing.T) {
	c := &Config{}
	c.QuickMenu = QuickMenu{Layout: "wheel", Position: "custom", X: 1.7, Y: -2, Monitor: "tv", Opacity: 0.1}
	c.Fix()
	q := c.QuickMenu
	if q.Layout != "list" || q.Position != "custom" || q.X != 1 || q.Y != 0 || q.Monitor != "game" || q.Opacity != 0.3 {
		t.Fatalf("%+v", q)
	}
	c.QuickMenu = QuickMenu{Layout: "strip", Position: "top-right", Monitor: "primary"}
	c.Fix()
	if q := c.QuickMenu; q.Layout != "strip" || q.Position != "top-right" || q.Monitor != "primary" || q.Opacity != 0.95 {
		t.Fatalf("%+v", q)
	}
	d, err := Load(writeTemp(t, `{"version": 3, "quick_menu": {"enabled": true, "position": "top"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if q := d.QuickMenu; q.Layout != "list" || q.Opacity != 0.95 || q.Monitor != "game" || q.Position != "top" {
		t.Fatalf("старый конфиг без новых полей: %+v", q)
	}
}

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFixWheel(t *testing.T) {
	c := &Config{}
	c.Wheel = Wheel{Items: []string{" sound:a ", "", "stop"}, Center: "middle", Scale: 5, Opacity: 0.1}
	c.Fix()
	w := c.Wheel
	if len(w.Items) != WheelSlots || w.Items[0] != "sound:a" || w.Items[1] != "" || w.Items[2] != "stop" || w.Items[7] != "" {
		t.Fatalf("свой набор должен стать ровно %d мест: %q", WheelSlots, w.Items)
	}
	if w.Center != "cursor" || w.Scale != 2 || w.Opacity != 0.3 {
		t.Fatalf("%+v", w)
	}
	c.Wheel = Wheel{Items: make([]string, 12), Center: "screen"}
	c.Wheel.Items[11] = "sound:z"
	c.Fix()
	if w := c.Wheel; len(w.Items) != WheelSlots || w.Center != "screen" || w.Scale != 1 || w.Opacity != 0.92 {
		t.Fatalf("%+v", w)
	}
	c.Wheel = Wheel{Items: []string{}}
	c.Fix()
	if c.Wheel.Items != nil {
		t.Fatalf("пустой список — автоподбор: %q", c.Wheel.Items)
	}
	d, err := Load(writeTemp(t, `{"version": 3}`))
	if err != nil {
		t.Fatal(err)
	}
	if w := d.Wheel; w.Enabled || w.Items != nil || w.Center != "cursor" || w.Scale != 1 || w.Opacity != 0.92 {
		t.Fatalf("старый конфиг без колеса: %+v", w)
	}
}

func TestFixRemote(t *testing.T) {
	c := &Config{}
	c.Remote = Remote{Port: 80, Sounds: "all", Key: "Bad Key!"}
	c.Fix()
	if r := c.Remote; r.Port != 0 || r.Sounds != "pad" || r.Key != "" {
		t.Fatalf("выключенный пульт не должен получать ключ: %+v", r)
	}
	c.Remote.Enabled = true
	c.Fix()
	k := c.Remote.Key
	if !ValidRemoteKey(k) || len(k) != 12 {
		t.Fatalf("включённому пульту нужен ключ: %q", k)
	}
	c.Fix()
	if c.Remote.Key != k {
		t.Fatal("ключ не должен меняться при каждом Fix")
	}
	c.Remote.Key = "  ABCDEFGH23  "
	c.Remote.Port, c.Remote.Sounds = 4000, "menu"
	c.Fix()
	if r := c.Remote; r.Key != "abcdefgh23" || r.Port != 4000 || r.Sounds != "menu" {
		t.Fatalf("%+v", r)
	}
	if p := c.RemotePort(); p != 4000 {
		t.Fatal(p)
	}
	c.Remote.Port = 0
	c.UI.Port = 5000
	if p := c.RemotePort(); p != 5001 {
		t.Fatal(p)
	}
	c.UI.Port = 0
	if p := c.RemotePort(); p != 3002 {
		t.Fatal(p)
	}
}

func TestRemoteKey(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		k := NewRemoteKey()
		if !ValidRemoteKey(k) || len(k) != 12 || seen[k] {
			t.Fatalf("%q", k)
		}
		seen[k] = true
	}
	for _, k := range []string{"", "abc", "abcdefg0", "abcdefgl", "abcdefgh/", strings.Repeat("a", 65)} {
		if ValidRemoteKey(k) {
			t.Fatalf("%q не должен подходить", k)
		}
	}
	if !ValidRemoteKey("abcdefgh") || !ValidRemoteKey(strings.Repeat("z", 64)) {
		t.Fatal("граничные длины")
	}
}

const v3Config = `{
  "version": 3,
  "mic": { "mute": false, "gate_db": -50 },
  "dota": { "enabled": true, "voice_to": "monitor", "volume": 1, "rosh": true, "reminders": [
    { "id": "wisdom", "label": "Руна мудрости", "enabled": true, "start": 420, "every": 420, "before": 20 },
    { "id": "power",  "label": "Руна силы", "enabled": true, "start": 360, "every": 120, "before": 10 },
    { "id": "lotus",  "label": "Лотосы", "enabled": true, "start": 180, "every": 180, "before": 10 },
    { "id": "mine",   "label": "Своё", "enabled": true, "start": 100, "every": 100, "before": 5 }
  ] }
}`

func TestMigrateV3(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(v3Config), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	rs := c.Dota.Reminders
	if len(rs) != 5 || rs[1].ID != "power" || len(rs[1].At) != 1 || rs[1].At[0] != 360 || rs[1].Every != 0 {
		t.Fatalf("руны силы — только на 6:00: %+v", rs)
	}
	if rs[2].ID != "support_runes" || rs[2].Before != 40 || rs[2].To != "both" || !rs[2].Enabled {
		t.Fatalf("напоминание саппортам: %+v", rs[2])
	}
	if rs[3].Until != 540 || rs[4].Until != 0 || rs[4].Every != 100 {
		t.Fatalf("лотосы до 9:00, своё не трогаем: %+v", rs)
	}
	if !c.Mic.Denoise || c.Mic.DenoiseLevel != 0.92 || c.Mic.GateDB != -50 || c.Background.Volume != 0.3 || !c.Background.OnlyTalking {
		t.Fatalf("новые настройки голоса: %+v %+v", c.Mic, c.Background)
	}
	// повторная загрузка не дублирует
	Save(p, c)
	c2, _ := Load(p)
	if len(c2.Dota.Reminders) != 5 {
		t.Fatalf("после сохранения: %d напоминаний", len(c2.Dota.Reminders))
	}
}

func TestSetsFix(t *testing.T) {
	c := Default()
	c.Sets = []SoundSet{{ID: "d", Name: " Dota ", Sounds: []string{"a", "a", "b"}, Games: []string{"Dota2.exe ", "dota2.exe"}}, {ID: "d", Name: "dup"}, {ID: "x", Name: ""}}
	c.ActiveSet = "nope"
	c.Fix()
	if len(c.Sets) != 1 || c.Sets[0].Name != "Dota" || len(c.Sets[0].Sounds) != 2 || len(c.Sets[0].Games) != 1 || c.Sets[0].Games[0] != "dota2.exe" || c.ActiveSet != "" {
		t.Fatalf("%+v %q", c.Sets, c.ActiveSet)
	}
	c.Mic.Timbre = "custom"
	c.Mic.EQ = []float64{30, -2}
	c.Fix()
	if len(c.Mic.EQ) != 5 || c.Mic.EQ[0] != 12 || c.Mic.EQ[1] != -2 {
		t.Fatalf("свой тембр: %v", c.Mic.EQ)
	}
}

// Напоминание пользователя не должно получать поля от напоминания по умолчанию с тем же номером.
func TestRemindersNoFieldLeak(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"version":4,"dota":{"reminders":[
		{"id":"wisdom","label":"Руна мудрости","enabled":true,"start":420,"every":420,"before":20},
		{"id":"water","label":"Водная руна","enabled":true,"at":[120,240],"before":10}]}}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if r := c.Dota.Reminders[1]; r.Start != 0 || r.Every != 0 || r.To != "" || r.Say != "" {
		t.Fatalf("поля протекли из напоминания по умолчанию: %+v", r)
	}
}
