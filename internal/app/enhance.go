package app

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Улучшение голоса: шумоподавление, тембр (эквалайзер + ровная громкость) и фон под голос.

// EnhanceOutput — то, что умеет audio.Engine (в тестах может не быть).
type EnhanceOutput interface {
	SetDenoise(on bool, strength float64) error
	DenoiseOn() bool
	SetTimbre(*dsp.Chain)
	SetBackground(dsp.Ambience)
	SetBackgroundMix(vol float64, whileTalking bool)
}

// TimbreSpecs — цепочка тембра из настроек (nil — без обработки).
func TimbreSpecs(cfg *config.Config) []dsp.Spec {
	switch t := cfg.Mic.Timbre; t {
	case "":
		return nil
	case "custom":
		return dsp.TimbreSpecs(cfg.Mic.EQ, cfg.Mic.Smooth)
	default:
		if tb, ok := dsp.FindTimbre(t); ok {
			return dsp.TimbreSpecs(tb.EQ, tb.Smooth)
		}
	}
	return nil
}

// applyEnhance применяет улучшение голоса; цепочку тембра и фон пересоздаёт, только если они изменились
// (иначе при каждом сохранении настроек фон начинался бы заново).
func (a *App) applyEnhance(cfg *config.Config, lib *sounds.Library) {
	e, ok := a.out.(EnhanceOutput)
	if !ok {
		return
	}
	a.enhMu.Lock()
	defer a.enhMu.Unlock()
	if err := e.SetDenoise(cfg.Mic.Denoise, cfg.Mic.DenoiseLevel); err != nil {
		log.Printf("Шумоподавление не включилось: %v", err)
	}
	specs := TimbreSpecs(cfg)
	key := ""
	if specs != nil {
		b, _ := json.Marshal(specs)
		key = string(b)
	}
	if key != a.enhTimbre {
		a.enhTimbre = key
		var ch *dsp.Chain
		if specs != nil {
			c, err := dsp.BuildChain("timbre", specs, sounds.SampleRate)
			if err != nil {
				log.Printf("Тембр: %v", err)
			} else {
				ch = c
			}
		}
		e.SetTimbre(ch)
	}
	bk := bgKey(cfg.Background.ID, lib)
	if bk != a.enhBG {
		a.enhBG = bk
		var g dsp.Ambience
		if bk != "" {
			var err error
			if g, err = newBackground(cfg.Background.ID, lib); err != nil {
				log.Printf("Фон: %v", err)
			}
		}
		e.SetBackground(g)
	}
	e.SetBackgroundMix(cfg.Background.Volume, cfg.Background.OnlyTalking)
}

// bgKey — что сейчас должно звучать фоном ("" — ничего); для своего звука учитывается и сам файл.
func bgKey(id string, lib *sounds.Library) string {
	sid, custom := strings.CutPrefix(id, "sound:")
	if !custom {
		return id
	}
	s := lib.Get(sid)
	if s == nil || len(s.Clips) == 0 {
		return ""
	}
	return fmt.Sprintf("%s#%d#%s", id, len(s.Clips[0]), strings.Join(s.Files, "|"))
}

// newBackground — генератор фона: встроенный (дождь, костёр…) или свой звук по кругу.
func newBackground(id string, lib *sounds.Library) (dsp.Ambience, error) {
	if sid, ok := strings.CutPrefix(id, "sound:"); ok {
		s := lib.Get(sid)
		if s == nil || len(s.Clips) == 0 {
			return nil, fmt.Errorf("звук %q не найден", sid)
		}
		if g := loopOf(s.Clips[0]); g != nil {
			return g, nil
		}
		return nil, fmt.Errorf("звук «%s» слишком короткий для фона", DisplayLabel(s))
	}
	if g := dsp.NewAmbience(id, sounds.SampleRate); g != nil {
		return g, nil
	}
	return nil, fmt.Errorf("нет фона %q", id)
}

// loopOf — свой звук по кругу; громкость приводится к уровню встроенных фонов, чтобы ползунок вёл себя одинаково.
func loopOf(c sounds.Clip) dsp.Ambience {
	if len(c) == 0 {
		return nil
	}
	var sum float64
	var peak float32
	for _, x := range c {
		sum += float64(x) * float64(x)
		peak = max(peak, x, -x)
	}
	g := 1.0
	if r := math.Sqrt(sum / float64(len(c))); r > 1e-5 {
		g = min(0.1/r, 4)
	}
	if float64(peak)*g > 0.95 {
		g = 0.95 / float64(peak)
	}
	data := make([]float32, len(c))
	for i, x := range c {
		data[i] = float32(float64(x) * g)
	}
	l := dsp.NewLoop(data, sounds.SampleRate)
	if l == nil {
		return nil
	}
	return l
}

// PreviewBackground — послушать фон самому (6 секунд в наушники); id "" — выбранный в настройках.
func (a *App) PreviewBackground(id string, volume float64) error {
	a.mu.Lock()
	cfg, lib := a.cfg, a.lib
	a.mu.Unlock()
	if id == "" {
		id = cfg.Background.ID
	}
	if id == "" {
		return fmt.Errorf("фон не выбран")
	}
	if volume <= 0 {
		volume = cfg.Background.Volume
	}
	g, err := newBackground(id, lib)
	if err != nil {
		return err
	}
	const sec = 6
	buf := make([]float32, sec*sounds.SampleRate)
	g.Render(buf)
	fade := int(0.4 * sounds.SampleRate)
	for i := 0; i < fade; i++ {
		k := float32(i) / float32(fade)
		buf[i] *= k
		buf[len(buf)-1-i] *= k
	}
	return a.playClip("preview", sounds.Clip(buf), float32(clampF(volume, 0.05, 1)), sounds.ModeInterrupt, sounds.BusMonitor)
}

// UpdateEnhance — изменить улучшение голоса без полной перезагрузки (ползунки эквалайзера, громкость фона).
func (a *App) UpdateEnhance(f func(c *config.Config)) error {
	return a.saveApply(f, func(c *config.Config) {
		a.mu.Lock()
		lib := a.lib
		a.mu.Unlock()
		a.applyEnhance(c, lib)
	})
}

// toggleDenoise — действие «шумоподавление вкл/выкл».
func (a *App) toggleDenoise() {
	var on bool
	a.UpdateEnhance(func(c *config.Config) { c.Mic.Denoise = !c.Mic.Denoise; on = c.Mic.Denoise })
	a.toast(Toast{Emoji: "🔇", Text: "Шумоподавление: " + onOff(on)})
	log.Printf("Шумоподавление: %s", onOff(on))
}

// toggleBackground — действие «фон вкл/выкл»: выключает фон или включает последний (по умолчанию — дождь).
func (a *App) toggleBackground() {
	var id string
	a.UpdateEnhance(func(c *config.Config) {
		b := &c.Background
		if b.ID != "" {
			b.Last, b.ID = b.ID, ""
		} else {
			b.ID = or(b.Last, "rain")
		}
		id = b.ID
	})
	if id == "" {
		a.toast(Toast{Emoji: "🔈", Text: "Фон выключен"})
		return
	}
	title, emoji := BackgroundTitle(id, a.Config())
	a.toast(Toast{Emoji: emoji, Text: "Фон: " + title})
}

// BackgroundTitle — название и значок фона.
func BackgroundTitle(id string, cfg *config.Config) (string, string) {
	if sid, ok := strings.CutPrefix(id, "sound:"); ok {
		d := cfg.Sounds[sid]
		return or(d.Label, sid), or(d.Emoji, "🔊")
	}
	for _, b := range dsp.Ambiences {
		if b.ID == id {
			return b.Title, b.Emoji
		}
	}
	return id, "🔊"
}

func onOff(on bool) string {
	if on {
		return "вкл"
	}
	return "выкл"
}
