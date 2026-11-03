package audio

import (
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/rnnoise"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Улучшение голоса: шумоподавление (до шумового порога), тембр (после «когда меня слышно»,
// до эффекта) и фон под голос (после эффекта, чтобы рация не превращала дождь в треск).

// SetDenoise включает шумоподавление; strength 0..1. Модель загружается при первом включении.
func (e *Engine) SetDenoise(on bool, strength float64) error {
	if on && e.dn.Load() == nil {
		s, err := rnnoise.NewStream()
		if err != nil {
			return err
		}
		e.dn.Store(s)
	}
	e.dnStr.Store(float32(strength))
	e.dnOn.Store(on)
	return nil
}

// DenoiseOn — работает ли шумоподавление.
func (e *Engine) DenoiseOn() bool { return e.dnOn.Load() && e.dn.Load() != nil }

// SetTimbre — цепочка тембра (nil — без обработки). Смена — с плавным переходом.
func (e *Engine) SetTimbre(c *dsp.Chain) { e.timbre.Store(c) }

// bgBox — фон для аудиопотока.
type bgBox struct{ g dsp.Ambience }

// SetBackground меняет фон (nil — без фона). Старый плавно уходит, новый входит (0,3 с).
func (e *Engine) SetBackground(g dsp.Ambience) {
	if g == nil {
		e.bg.Store(nil)
		return
	}
	e.bg.Store(&bgBox{g: g})
}

// SetBackgroundMix — громкость фона 0..1 и «только пока я говорю».
func (e *Engine) SetBackgroundMix(vol float64, whileTalking bool) {
	e.bgVol.Store(float32(vol))
	e.bgTalk.Store(whileTalking)
}

// bgMix — состояние фона в аудиопотоке.
type bgMix struct {
	cur, old  *bgBox
	xf        int
	gain      float32
	hold      int
	buf, obuf []float32
}

const (
	bgXF     = sounds.SampleRate * 3 / 10 // смена фона — 0,3 с
	bgHold   = sounds.SampleRate * 9 / 10 // фон держится 0,9 с после паузы в речи
	bgAttack = 1.0 / (sounds.SampleRate * 0.08)
	bgRel    = 1.0 / (sounds.SampleRate * 0.6)
)

// talking — говорит ли человек сейчас: по кнопке (режим «пока держу»/«вкл-выкл»),
// по шумовому порогу или по уровню очищенного голоса.
func (e *Engine) talking() bool {
	if e.micMute.Load() {
		return false
	}
	if e.talkGate.Load() {
		return e.talkOpen.Load()
	}
	if e.gateThr.Load() > 0 {
		return e.gateHold > 0
	}
	return e.micEnv > 0.012 // ≈ −38 дБ
}

func (e *Engine) mixBackground(b []float32) {
	m := &e.bgState
	if box := e.bg.Load(); box != m.cur {
		m.old, m.cur, m.xf = m.cur, box, 0
	}
	n := len(b)
	target := float32(1)
	switch {
	case e.micMute.Load(), e.talkGate.Load() && !e.talkOpen.Load():
		target, m.hold = 0, 0 // микрофон выключен или кнопка «говорить» отпущена — фона тоже нет
	case e.bgTalk.Load():
		if e.talking() {
			m.hold = bgHold
		} else if m.hold > 0 {
			m.hold -= n
		}
		if m.hold <= 0 {
			target = 0
		}
	}
	if m.cur == nil && m.old == nil || target == 0 && m.gain < 1e-4 && m.old == nil {
		m.gain = 0 // тишина — генератор не крутим
		return
	}
	m.buf = growNoClear(m.buf, n)
	if m.cur != nil {
		m.cur.g.Render(m.buf)
	} else {
		clear(m.buf)
	}
	if m.old != nil {
		m.obuf = growNoClear(m.obuf, n)
		m.old.g.Render(m.obuf)
		for i := range m.buf {
			t := float32(m.xf+i) / bgXF
			if t > 1 {
				t = 1
			}
			m.buf[i] = m.buf[i]*t + m.obuf[i]*(1-t)
		}
		if m.xf += n; m.xf >= bgXF {
			m.old = nil
		}
	}
	vol := e.bgVol.Load()
	for i := range b {
		if target > m.gain {
			m.gain = min(target, m.gain+bgAttack)
		} else {
			m.gain = max(target, m.gain-bgRel)
		}
		b[i] += m.buf[i] * m.gain * vol
	}
}
