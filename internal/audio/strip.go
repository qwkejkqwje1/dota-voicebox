package audio

import (
	"math"
	"sync/atomic"
)

// SetMicStrip — mute микрофона и порог шумового гейта (dBFS, 0 = выключен).
func (e *Engine) SetMicStrip(mute bool, gateDB float64) {
	e.micMute.Store(mute)
	if gateDB >= 0 || gateDB < -90 {
		e.gateThr.Store(0)
	} else {
		e.gateThr.Store(float32(math.Pow(10, gateDB/20)))
	}
}

// gate — шумовой гейт с удержанием: тихий фон между фразами не уходит в эфир.
func (e *Engine) gate(b []float32) {
	thr := e.gateThr.Load()
	const hold = 48000 * 250 / 1000 // 250 мс
	for i, x := range b {
		a := abs32(x)
		// огибающая: быстрая атака, медленный спад
		if a > e.micEnv {
			e.micEnv = a
		} else {
			e.micEnv *= 0.9995
		}
		if thr == 0 {
			continue
		}
		if e.micEnv >= thr {
			e.gateHold = hold
		} else if e.gateHold > 0 {
			e.gateHold--
		}
		if e.gateHold > 0 {
			e.gateGain += (1 - e.gateGain) * 0.02 // ~1 мс
		} else {
			e.gateGain *= 0.9985 // ~15 мс
		}
		b[i] = x * e.gateGain
	}
	if thr == 0 {
		e.gateGain = 1
	}
}

// GateOpen — открыт ли гейт сейчас (для индикатора).
func (e *Engine) GateOpen() bool { return e.gateThr.Load() == 0 || e.gateHold > 0 }

// tap — «отвод» последних сэмплов для визуализации (перезаписывается по кругу).
type tap struct {
	buf [8192]float32
	w   atomic.Uint64
}

func (t *tap) write(p []float32) {
	w := t.w.Load()
	for _, x := range p {
		t.buf[w&8191] = x
		w++
	}
	t.w.Store(w)
}

// Snapshot копирует последние len(dst) сэмплов (до 8192).
func (t *tap) Snapshot(dst []float32) {
	w := t.w.Load()
	n := uint64(len(dst))
	for i := uint64(0); i < n; i++ {
		dst[i] = t.buf[(w-n+i)&8191]
	}
}

// VizSnapshot — последние сэмплы микрофона до эффектов (post=false) или эфира после (post=true).
func (e *Engine) VizSnapshot(dst []float32, post bool) {
	if post {
		e.vizPost.Snapshot(dst)
	} else {
		e.vizPre.Snapshot(dst)
	}
}
