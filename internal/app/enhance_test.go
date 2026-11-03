package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
)

// enhOut — движок с улучшением голоса (для тестов).
type enhOut struct {
	fakeOut
	denoise  bool
	level    float64
	timbre   *dsp.Chain
	timbres  int
	bg       dsp.Ambience
	bgs      int
	vol      float64
	onlyTalk bool
}

func (e *enhOut) SetDenoise(on bool, s float64) error { e.denoise, e.level = on, s; return nil }
func (e *enhOut) DenoiseOn() bool                     { return e.denoise }
func (e *enhOut) SetTimbre(c *dsp.Chain)              { e.timbre = c; e.timbres++ }
func (e *enhOut) SetBackground(g dsp.Ambience)        { e.bg = g; e.bgs++ }
func (e *enhOut) SetBackgroundMix(v float64, t bool)  { e.vol, e.onlyTalk = v, t }

func TestEnhance(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, config.DefaultJSON, 0o644)
	out := &enhOut{}
	a := New(p, out)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	// по умолчанию: шумоподавление включено, тембра и фона нет
	if !out.denoise || out.level != 0.92 || out.timbre != nil || out.bg != nil || !a.Status().Denoise {
		t.Fatalf("по умолчанию: %+v", out)
	}
	a.UpdateEnhance(func(c *config.Config) { c.Mic.Timbre = "radio"; c.Background.ID = "rain"; c.Background.Volume = 0.4 })
	if out.timbre == nil || out.bg == nil || out.vol != 0.4 || !out.onlyTalk {
		t.Fatalf("тембр и фон: %+v", out)
	}
	n, bgs := out.timbres, out.bgs
	a.Reload() // ничего не поменялось — фон не начинается заново
	a.UpdateEnhance(func(c *config.Config) { c.Background.Volume = 0.2 })
	if out.timbres != n || out.bgs != bgs || out.vol != 0.2 {
		t.Fatalf("лишняя пересборка: тембр %d→%d, фон %d→%d", n, out.timbres, bgs, out.bgs)
	}
	// свой тембр
	a.UpdateEnhance(func(c *config.Config) {
		c.Mic.Timbre = "custom"
		c.Mic.EQ = []float64{3, 0, 0, 2, 0}
		c.Mic.Smooth = 0.5
	})
	if out.timbres != n+1 || len(TimbreSpecs(a.Config())) != 5 { // срез + 2 полосы + компрессор + ограничитель
		t.Fatalf("свой тембр: %v", TimbreSpecs(a.Config()))
	}
	// действия: шумоподавление и фон вкл/выкл (фон помнит последний)
	a.Do("denoise")
	if out.denoise || a.Config().Mic.Denoise {
		t.Fatal("denoise не выключил")
	}
	a.Do("bg")
	if out.bg != nil || a.Config().Background.Last != "rain" {
		t.Fatalf("bg не выключил: %+v", a.Config().Background)
	}
	a.Do("bg")
	if out.bg == nil || a.Config().Background.ID != "rain" {
		t.Fatal("bg не включил последний фон")
	}
	// свой звук фоном
	a.UpdateEnhance(func(c *config.Config) { c.Background.ID = "sound:drumroll" })
	if _, ok := out.bg.(*dsp.Loop); !ok {
		t.Fatalf("свой звук фоном: %T", out.bg)
	}
	buf := make([]float32, 48000)
	out.bg.Render(buf)
	if err := a.PreviewBackground("storm", 0.5); err != nil || !out.m.Active() || out.v.Active() {
		t.Fatal("прослушивание фона — только в наушники", err)
	}
	if err := a.PreviewBackground("nope", 0); err == nil {
		t.Fatal("несуществующий фон")
	}
	a.DeleteSound("drumroll")
	if a.Config().Background.ID != "" || out.bg != nil {
		t.Fatal("удалённый звук остался фоном")
	}
}
