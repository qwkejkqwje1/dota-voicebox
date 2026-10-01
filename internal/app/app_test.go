package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
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

func newApp(t *testing.T) (*App, *fakeOut) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, config.DefaultJSON, 0o644)
	out := &fakeOut{}
	a := New(p, out)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	return a, out
}

func TestHotkeyActions(t *testing.T) {
	a, out := newApp(t)
	a.Do("sound:siren")
	if !out.v.Active() || !out.m.Active() {
		t.Fatal("сирена должна играть в войс и в наушники")
	}
	a.Do("stop")
	a.Do("sound:wisdom_rune")
	if out.v.Active() || !out.m.Active() {
		t.Fatal("руна по умолчанию — только в наушники")
	}
	a.Do("preset:dead_inside")
	if out.chain.Name != "dead_inside" {
		t.Fatal(out.chain.Name)
	}
	a.Do("preset:next")
	if out.chain.Name != "dead_inside_max" {
		t.Fatal(out.chain.Name)
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

func TestGSITimers(t *testing.T) {
	a, out := newApp(t)
	for c := 340; c < 350; c++ {
		a.OnGSI(mk(c))
	}
	if out.m.Active() {
		t.Fatal("рано")
	}
	st := a.Status()
	if !st.Game.Connected || len(st.Upcoming) == 0 || st.Upcoming[0].ID != "mid_rune_6" || st.Upcoming[0].In != 11 {
		t.Fatalf("%+v", st)
	}
	a.OnGSI(mk(350)) // 6:00 - 10с
	if !out.m.Active() {
		t.Fatal("нет звука руны на 6:00")
	}
}

func TestSaveConfigValidatesAndRestartsOnlyOnDeviceChange(t *testing.T) {
	a, out := newApp(t)
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
