package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

type fakeOut struct {
	v, m   audio.Mixer
	chain  *dsp.Chain
	starts int
}

func (f *fakeOut) Voice() *audio.Mixer                             { return &f.v }
func (f *fakeOut) Monitor() *audio.Mixer                           { return &f.m }
func (f *fakeOut) SetChain(c *dsp.Chain)                           { f.chain = c }
func (f *fakeOut) ToggleMicMonitor() bool                          { return true }
func (f *fakeOut) MicMonitor() bool                                { return false }
func (f *fakeOut) Running() bool                                   { return true }
func (f *fakeOut) Start(audio.Options) error                       { f.starts++; return nil }
func (f *fakeOut) SetMix(float64, float64, float64, float64, bool) {}
func (f *fakeOut) Status() audio.Status                            { return audio.Status{Running: true} }

func newApp(t *testing.T, speech bool) (*App, *fakeOut) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, config.DefaultJSON, 0o644)
	out := &fakeOut{}
	a := New(p, out)
	if speech {
		a.speech = &fakeSpeech{mem: map[string]sounds.Clip{}, rate: 1}
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	return a, out
}

// waitSpeech ждёт окончания фонового синтеза речи.
func waitSpeech(t *testing.T, a *App) {
	t.Helper()
	for i := 0; i < 300 && a.ttsBusy.Load(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if a.ttsBusy.Load() {
		t.Fatal("синтез речи не закончился")
	}
}

func TestPlayToggleStopAndPresets(t *testing.T) {
	a, out := newApp(t, false)
	a.Do("sound:airhorn")
	if !out.v.Active() || !out.m.Active() || !a.IsPlaying("airhorn") {
		t.Fatal("звук должен играть команде и вам")
	}
	if p := a.Playing(); len(p) != 1 || p[0].ID != "airhorn" || p[0].Len < 1 {
		t.Fatalf("%+v", p)
	}
	a.Do("sound:airhorn") // режим toggle: повторное нажатие — стоп
	if a.IsPlaying("airhorn") {
		t.Fatal("повторное нажатие должно остановить звук")
	}
	a.Do("sound:laser")
	a.Do("stop")
	if a.IsPlaying("laser") {
		t.Fatal("stop")
	}
	a.Do("preset:dead_inside")
	if out.chain.Name != "dead_inside" {
		t.Fatal(out.chain.Name)
	}
	a.Do("preset:next")
	if out.chain.Name != "dead_inside_max" {
		t.Fatal(out.chain.Name)
	}
	if err := a.SetPresetParams("radio", map[string]float64{"noise": 0, "beep": 4}); err != nil {
		t.Fatal(err)
	}
	if v := a.Config().PresetParams["radio"]; v["noise"] != 0 || v["beep"] != 4 || v["drive"] != 40 {
		t.Fatalf("ручки: %v", v)
	}
}

func mk(clock int) *gsi.State {
	s := &gsi.State{}
	s.Map = &struct {
		Name              string `json:"name"`
		MatchID           string `json:"matchid"`
		GameTime          int    `json:"game_time"`
		ClockTime         int    `json:"clock_time"`
		Daytime           bool   `json:"daytime"`
		NightstalkerNight bool   `json:"nightstalker_night"`
		GameState         string `json:"game_state"`
		Paused            bool   `json:"paused"`
		WinTeam           string `json:"win_team"`
	}{MatchID: "1", ClockTime: clock, GameState: gsi.InProgress}
	return s
}

func TestDotaModule(t *testing.T) {
	a, out := newApp(t, true)
	// выключен: нет кнопок Dota, клавиш и напоминаний
	if a.lib.Get("dota_gank") != nil || a.actions["Num1"] != "" || len(a.timers.Upcoming()) != 0 {
		t.Fatal("модуль выключен, но его звуки загружены")
	}
	for _, id := range a.PadOrder() {
		if strings.HasPrefix(id, "dota_") {
			t.Fatal("кнопка Dota на панели при выключенном модуле")
		}
	}
	d := a.Config().Dota
	d.Enabled = true
	if err := a.SetDota(d); err != nil {
		t.Fatal(err)
	}
	waitSpeech(t, a)
	g := a.lib.Get("dota_gank")
	if g == nil || g.Pending || len(g.Clips) != 1 || g.Err != "" {
		t.Fatalf("кнопка ганка: %+v", g)
	}
	if a.actions["Num1"] != "sound:dota_gank" {
		t.Fatalf("клавиши: %v", a.actions)
	}
	if r := a.lib.Get(reminderSound("wisdom")); r == nil || len(r.Clips) != 1 || r.Bus != sounds.BusMonitor {
		t.Fatalf("напоминание: %+v", r)
	}
	// саппортам — за 40 секунд до рун силы, слышит и команда
	if r := a.lib.Get(reminderSound("support_runes")); r == nil || r.Bus != sounds.BusBoth {
		t.Fatalf("саппорты к рунам: %+v", r)
	}
	for _, r := range a.Config().Dota.Reminders {
		if r.ID == "support_runes" {
			if f := reminderDef(a.Config(), r).Files; f[len(f)-1] != "tts:Саппорты, руны силы через 40 секунд. Перетянитесь и подконтрольте руны" {
				t.Fatal(f)
			}
		}
	}
	for c := 340; c < 350; c++ {
		a.OnGSI(mk(c), nil)
	}
	if out.m.Active() {
		t.Fatal("рано")
	}
	st := a.Status()
	if !st.Game.Connected || len(st.Upcoming) == 0 || st.Upcoming[0].ID != "power" || st.Upcoming[0].Label != "Руны силы" {
		t.Fatalf("%+v", st.Upcoming)
	}
	a.OnGSI(mk(350), nil) // 6:00 − 10 с
	if !a.IsPlaying(reminderSound("power")) || out.v.Active() {
		t.Fatal("напоминание о руне силы должно звучать только у вас")
	}
	// выключаем — GSI и звуки уходят
	d.Enabled = false
	a.SetDota(d)
	if a.lib.Get("dota_gank") != nil || a.gsiSrv != nil {
		t.Fatal("модуль не выключился")
	}
}

func TestReminderPhrases(t *testing.T) {
	cases := map[int]string{1: "1 секунду", 2: "2 секунды", 5: "5 секунд", 11: "11 секунд", 21: "21 секунду", 22: "22 секунды",
		30: "30 секунд", 60: "минуту", 90: "минуту 30 секунд", 120: "2 минуты", 300: "5 минут"}
	for n, want := range cases {
		if got := Seconds(n); got != want {
			t.Errorf("%d: %q, нужно %q", n, got, want)
		}
	}
	if p := ReminderPhrase(config.Reminder{Label: "Руна мудрости", Before: 20}); p != "Руна мудрости через 20 секунд" {
		t.Fatal(p)
	}
	if p := ReminderPhrase(config.Reminder{Label: "Стак", Say: "Стак!", Before: 7}); p != "Стак!" {
		t.Fatal(p)
	}
	if p := ReminderPhrase(config.Reminder{Label: "x", Say: "-"}); p != "" {
		t.Fatal(p)
	}
	if p := ReminderPhrase(config.Reminder{Say: "Саппорты, руны {время}.", Before: 40}); p != "Саппорты, руны через 40 секунд." {
		t.Fatal(p)
	}
	if p := ReminderPhrase(config.Reminder{Say: "Руны {время}", Before: 0}); p != "Руны сейчас" {
		t.Fatal(p)
	}
	if Slug("Мид, ганк!") != "mid_gank" || Slug("!!!") != "sound" || Slug("Hello World 2") != "hello_world_2" {
		t.Fatal(Slug("Мид, ганк!"), Slug("!!!"))
	}
}

func TestSoundManagement(t *testing.T) {
	a, _ := newApp(t, true)
	beep, _ := sounds.Builtin("beep")
	id, err := a.ImportSound("Мой смех.wav", sounds.EncodeWAV16(beep), ".wav")
	if err != nil || id != "moy_smeh" {
		t.Fatal(id, err)
	}
	if _, err := os.Stat(filepath.Join(a.SoundsDir(), "moy_smeh.wav")); err != nil {
		t.Fatal(err)
	}
	pad := a.PadOrder()
	if pad[len(pad)-1] != id || a.lib.Get(id) == nil || a.lib.Get(id).Label != "Мой смех" {
		t.Fatalf("панель: %v", pad)
	}
	if _, err := a.ImportSound("bad.wav", []byte("nope"), ".wav"); err == nil {
		t.Fatal("битый файл принят")
	}
	key := "Ctrl+Num5"
	inMenu := true
	err = a.UpdateSound(id, SoundEdit{Label: "Смех", Emoji: "😂", Color: "#ff0000", Bus: "voice", Volume: 0.8, Mode: "overlap",
		Hotkey: &key, InMenu: &inMenu})
	if err != nil {
		t.Fatal(err)
	}
	s := a.lib.Get(id)
	if s.Label != "Смех" || s.Emoji != "😂" || s.Bus != sounds.BusVoice || s.Mode != "overlap" || a.HotkeyMap()["sound:"+id] != key {
		t.Fatalf("%+v %v", s, a.HotkeyMap())
	}
	m := a.Menu()
	if len(m.Sounds) == 0 || m.Sounds[len(m.Sounds)-1].ID != id || m.Sounds[len(m.Sounds)-1].Key != key || len(m.Effects) != 10 {
		t.Fatalf("меню: %+v", m)
	}
	// клавиша, занятая другим действием, переходит
	if err := a.BindHotkey("stop", key); err != nil || a.HotkeyMap()["sound:"+id] != "" || a.HotkeyMap()["stop"] != key {
		t.Fatal(err, a.HotkeyMap())
	}
	// фраза: синтез в фоне
	pid, err := a.AddPhrase("Всем привет!", "")
	if err != nil {
		t.Fatal(err)
	}
	waitSpeech(t, a)
	if p := a.lib.Get(pid); p == nil || p.Pending || len(p.Clips) != 1 {
		t.Fatalf("фраза: %+v", p)
	}
	bid, err := a.AddBuiltin("siren")
	if err != nil || a.lib.Get(bid).Emoji != "🚨" {
		t.Fatal(bid, err)
	}
	// удаление: файл в корзину, клавиши и меню чистятся
	a.BindHotkey("sound:"+id, "Ctrl+Num6")
	if err := a.DeleteSound(id); err != nil {
		t.Fatal(err)
	}
	if a.lib.Get(id) != nil || a.HotkeyMap()["sound:"+id] != "" {
		t.Fatal("звук не удалён")
	}
	if _, err := os.Stat(filepath.Join(a.BaseDir, "trash", "moy_smeh.wav")); err != nil {
		t.Fatal("файл не в корзине:", err)
	}
	for _, x := range a.Config().QuickMenu.Items {
		if x == id {
			t.Fatal("удалённый звук остался в меню")
		}
	}
	// порядок
	order := a.PadOrder()
	order[0], order[1] = order[1], order[0]
	a.SetPad(order)
	if got := a.PadOrder(); got[0] != order[0] {
		t.Fatal("порядок не сохранился")
	}
}

func TestSaveConfigValidatesAndRestartsOnlyOnDeviceChange(t *testing.T) {
	a, out := newApp(t, false)
	starts := out.starts
	if err := a.UpdateConfig(func(c *config.Config) { c.MicGain = 1.5 }); err != nil {
		t.Fatal(err)
	}
	if out.starts != starts {
		t.Fatal("лишний перезапуск аудио")
	}
	a.UpdateConfig(func(c *config.Config) { c.Devices.Mic = "USB" })
	if out.starts != starts+1 {
		t.Fatal("нет перезапуска при смене устройства")
	}
	err := a.UpdateConfig(func(c *config.Config) { c.Hotkeys["Hyper+Q"] = "stop" })
	if err == nil {
		t.Fatal("битая клавиша сохранилась")
	}
}

func TestWheel(t *testing.T) {
	a, out := newApp(t, false)
	w := a.Wheel()
	if !w.Auto || len(w.Items) != config.WheelSlots || w.Items[0].Act != "sound:airhorn" || w.Items[0].Label != "Дудка" {
		t.Fatalf("автоподбор — первые 8 звуков с панели: %+v", w)
	}
	var opened []uint32
	a.OnWheel = func(vk uint32) { opened = append(opened, vk) }
	a.Do("wheel")
	a.doKey("wheel", 0x56)
	if len(opened) != 2 || opened[0] != 0 || opened[1] != 0x56 {
		t.Fatalf("%v", opened)
	}
	set := a.Config().Wheel
	set.Enabled = true
	set.Items = []string{"sound:airhorn", "preset:dead_inside", "stop", "mic_mute", "sound:nope", "", "preset:nope", ""}
	if err := a.SetWheel(set); err != nil {
		t.Fatal(err)
	}
	if err := a.BindHotkey("wheel", "V"); err != nil {
		t.Fatal(err)
	}
	w = a.Wheel()
	if w.Auto || !w.Settings.Enabled || w.Key == "" {
		t.Fatalf("%+v", w)
	}
	it := w.Items
	if it[1].Label == "" || it[2].Label != "Стоп" || it[3].Active || !it[4].Missing || it[5].Act != "" || !it[6].Missing {
		t.Fatalf("%+v", it)
	}
	a.WheelPick("sound:airhorn")
	if !a.IsPlaying("airhorn") || !a.Wheel().Items[0].Active {
		t.Fatal("звук из колеса должен играть и подсвечиваться")
	}
	a.WheelPick("stop")
	a.WheelPick("preset:dead_inside")
	if a.IsPlaying("airhorn") || out.chain.Name != "dead_inside" || !a.Wheel().Items[1].Active {
		t.Fatal("стоп и эффект из колеса")
	}
	a.WheelPick("mic_mute")
	if !a.Config().Mic.Mute || !a.Wheel().Items[3].Active {
		t.Fatal("микрофон из колеса")
	}
	a.WheelPick("")
	set.Enabled = false
	if err := a.SetWheel(set); err != nil {
		t.Fatal(err)
	}
	if a.Config().Wheel.Enabled {
		t.Fatal("выключение колеса")
	}
}
