package sounds

import (
	"math"
	"math/rand"

	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
)

// Стартовый набор: звуки синтезируются в коде (никаких чужих записей и лицензий).

func init() {
	for k, f := range map[string]func() Clip{
		"airhorn":      airhorn,
		"sad_trombone": sadTrombone,
		"rimshot":      rimshot,
		"drumroll":     drumroll,
		"crickets":     crickets,
		"ding":         ding,
		"buzzer":       buzzer,
		"laser":        laser,
		"boing":        boing,
		"chime":        chime,
		"alert":        alert,
		"beep2":        beep2,
	} {
		builtins[k] = f
	}
}

// saw — пилообразная волна из гармоник до maxHz (без алиасинга).
func saw(ph, f, maxHz float64) float64 {
	var v float64
	for h := 1.0; h*f < maxHz && h < 60; h++ {
		v += math.Sin(h*ph) / h
	}
	return v
}

func filt(c Clip, kind string, f, q, db float64) {
	dsp.NewBiquad(kind, f, q, db, sr).Process(c)
}

func addAt(dst, src Clip, at float64, gain float32) {
	o := int(at * sr)
	for i := 0; i < len(src) && o+i < len(dst); i++ {
		if o+i >= 0 {
			dst[o+i] += src[i] * gain
		}
	}
}

// airhorn — «эйрхорн»: три коротких гудка и длинный.
func airhorn() Clip {
	c := buf(1.75)
	blasts := []struct{ at, d float64 }{{0, 0.16}, {0.22, 0.16}, {0.44, 0.16}, {0.66, 1.0}}
	freqs := []float64{438, 442.5, 446, 553}
	ph := make([]float64, len(freqs))
	for _, b := range blasts {
		s, n := int(b.at*sr), int((b.d+0.06)*sr)
		for i := 0; i < n && s+i < len(c); i++ {
			t := float64(i) / sr
			env := math.Min(1, t/0.012) * math.Min(1, math.Max(0, b.d+0.05-t)/0.05)
			bend := 1 - 0.04*math.Exp(-t/0.03)
			var v float64
			for k, f := range freqs {
				ph[k] += 2 * math.Pi * f * bend / sr
				amp := 1.0
				if k == 3 {
					amp = 0.5
				}
				v += saw(ph[k], f, 7000) * amp
			}
			c[s+i] += float32(math.Tanh(v*0.9) * env)
		}
	}
	filt(c, "peaking", 1250, 1.2, 9)
	filt(c, "peaking", 2600, 2, 5)
	filt(c, "highpass", 250, 0.7, 0)
	return normalize(c, 0.9)
}

// sadTrombone — «вах-вах-вах-ваааа» с сурдиной и вибрато на последней ноте.
func sadTrombone() Clip {
	notes := []struct{ f, d float64 }{{233.1, 0.42}, {220, 0.42}, {207.7, 0.42}, {196, 1.35}}
	c := buf(2.9)
	at := 0.0
	ph := 0.0
	for k, nt := range notes {
		s, n := int(at*sr), int((nt.d+0.08)*sr)
		seg := make(Clip, n)
		lp := dsp.NewBiquad("lowpass", 600, 3, 0, sr)
		for i := range seg {
			t := float64(i) / sr
			f := nt.f
			if k == 3 && t > 0.25 {
				f *= 1 + 0.018*math.Sin(2*math.Pi*5.2*(t-0.25))
			}
			ph += 2 * math.Pi * f / sr
			seg[i] = float32(saw(ph, f, 5000) * 0.5)
			if i%32 == 0 { // «вах»: сурдина открывается и закрывается
				w := math.Sin(math.Pi * math.Min(1, t/(nt.d*0.85)))
				lp = retune(lp, 350+1500*w)
			}
			seg[i] = float32(lp.Tick(float64(seg[i])))
			env := math.Min(1, t/0.03) * math.Min(1, math.Max(0, nt.d+0.06-t)/0.08)
			seg[i] *= float32(env)
		}
		addAt(c, seg, at, 1)
		at += nt.d + 0.04
		_ = s
	}
	return normalize(fadeOut(c, 0.2), 0.85)
}

// retune меняет частоту фильтра, сохраняя его состояние.
func retune(f *dsp.Biquad, hz float64) *dsp.Biquad {
	n := dsp.NewBiquad("lowpass", hz, 3, 0, sr)
	n.CopyState(f)
	return n
}

func noiseBurst(sec, decay float64, hp, lp float64) Clip {
	c := buf(sec)
	for i := range c {
		t := float64(i) / sr
		c[i] = float32((rand.Float64()*2 - 1) * math.Exp(-t/decay))
	}
	if hp > 0 {
		filt(c, "highpass", hp, 0.7, 0)
	}
	if lp > 0 {
		filt(c, "lowpass", lp, 0.7, 0)
	}
	return c
}

func drum(sec, f0, f1, decay float64) Clip {
	c := buf(sec)
	ph := 0.0
	for i := range c {
		t := float64(i) / sr
		f := f1 + (f0-f1)*math.Exp(-t/0.04)
		ph += 2 * math.Pi * f / sr
		c[i] = float32(math.Sin(ph) * math.Exp(-t/decay))
	}
	return c
}

func snare(gain float32) Clip {
	s := noiseBurst(0.25, 0.06, 900, 7000)
	body := drum(0.2, 260, 180, 0.05)
	for i := range s {
		if i < len(body) {
			s[i] += body[i] * 0.6
		}
		s[i] *= gain
	}
	return s
}

// cymbal — тарелка: шесть «квадратов» как у TR-808 плюс шум.
func cymbal(sec, decay float64) Clip {
	c := buf(sec)
	fs := []float64{205.3, 304.4, 369.6, 522.7, 540, 800}
	for i := range c {
		t := float64(i) / sr
		var v float64
		for _, f := range fs {
			if math.Sin(2*math.Pi*f*1.7*t) > 0 {
				v++
			} else {
				v--
			}
		}
		v = v/6 + (rand.Float64()*2-1)*0.6
		c[i] = float32(v * math.Exp(-t/decay))
	}
	filt(c, "highpass", 5500, 0.7, 0)
	filt(c, "peaking", 9000, 1, 4)
	return c
}

// rimshot — «ба-дум-тсс».
func rimshot() Clip {
	c := buf(1.7)
	addAt(c, snare(0.8), 0, 1)
	addAt(c, drum(0.5, 170, 95, 0.22), 0.2, 1)
	addAt(c, drum(0.5, 120, 60, 0.3), 0.2, 0.6)
	addAt(c, cymbal(1.3, 0.35), 0.42, 0.9)
	addAt(c, drum(0.5, 120, 55, 0.25), 0.42, 0.8)
	return normalize(fadeOut(c, 0.25), 0.85)
}

// drumroll — барабанная дробь с нарастанием и удар тарелки.
func drumroll() Clip {
	c := buf(3.2)
	for t := 0.0; t < 1.8; t += 1.0 / 26 {
		g := float32(0.25 + 0.75*t/1.8)
		addAt(c, snare(g*(0.85+0.3*float32(rand.Float64()))), t, 1)
	}
	addAt(c, cymbal(1.4, 0.5), 1.85, 1)
	addAt(c, drum(0.6, 110, 50, 0.3), 1.85, 1)
	return normalize(fadeOut(c, 0.3), 0.85)
}

// crickets — неловкая тишина: сверчки ночью.
func crickets() Clip {
	c := buf(3.4)
	for k, cr := range []struct{ f, period, off float64 }{{4500, 0.62, 0}, {4100, 0.71, 0.27}} {
		for at := cr.off; at < 3.2; at += cr.period {
			for p := 0; p < 3; p++ {
				s, n := int((at+float64(p)*0.045)*sr), int(0.022*sr)
				for i := 0; i < n && s+i < len(c); i++ {
					t := float64(i) / sr
					env := math.Sin(math.Pi * t / 0.022)
					c[s+i] += float32(0.35 * env * math.Sin(2*math.Pi*cr.f*float64(s+i)/sr) * (1 - 0.3*float64(k)))
				}
			}
		}
	}
	amb := noiseBurst(3.4, 1e9, 0, 900)
	for i := range c {
		c[i] += amb[i] * 0.02
	}
	return normalize(fadeOut(c, 0.4), 0.7)
}

// ding — «правильно!»: два колокольчика вверх.
func ding() Clip {
	c := buf(1.6)
	bell(c, 0, 1318.5, 0.6, 0.5)
	bell(c, 0.13, 1760, 0.7, 0.9)
	return normalize(fadeOut(c, 0.3), 0.85)
}

// buzzer — «ошибка»: грубый гудок из телевикторины.
func buzzer() Clip {
	c := buf(1.0)
	ph1, ph2 := 0.0, 0.0
	for i := range c {
		t := float64(i) / sr
		ph1 += 2 * math.Pi * 147 * (1 - 0.03*t) / sr
		ph2 += 2 * math.Pi * 153 * (1 - 0.03*t) / sr
		v := saw(ph1, 147, 6000) + saw(ph2, 153, 6000)
		env := math.Min(1, t/0.01) * math.Min(1, math.Max(0, 0.95-t)/0.06)
		c[i] = float32(math.Tanh(v*1.5) * env)
	}
	filt(c, "peaking", 900, 1, 6)
	return normalize(c, 0.8)
}

// laser — «пиу-пиу».
func laser() Clip {
	c := buf(0.7)
	for _, at := range []float64{0, 0.28} {
		s, n := int(at*sr), int(0.26*sr)
		ph := 0.0
		for i := 0; i < n && s+i < len(c); i++ {
			t := float64(i) / sr
			f := 180 + 2200*math.Exp(-t/0.06)
			ph += 2 * math.Pi * f / sr
			v := math.Sin(ph + 0.8*math.Sin(ph*0.5))
			env := math.Exp(-t/0.12) * math.Min(1, t/0.002) * math.Min(1, (0.26-t)/0.03)
			c[s+i] += float32(v * env)
		}
	}
	return normalize(c, 0.8)
}

// boing — пружина.
func boing() Clip {
	c := buf(1.1)
	ph := 0.0
	for i := range c {
		t := float64(i) / sr
		f := 170 + 230*(1-math.Exp(-t/0.05)) + 70*math.Sin(2*math.Pi*13*t)*math.Exp(-t/0.3)
		ph += 2 * math.Pi * f / sr
		v := math.Sin(ph) + 0.3*math.Sin(2*ph) + 0.15*math.Sin(3*ph)
		c[i] = float32(v * math.Exp(-t/0.33) * math.Min(1, t/0.004))
	}
	return normalize(c, 0.85)
}

// chime — мягкий сигнал перед голосовым напоминанием.
func chime() Clip {
	c := buf(0.9)
	bell(c, 0, 784, 0.45, 0.35)
	bell(c, 0.12, 1046.5, 0.45, 0.5)
	return normalize(fadeOut(c, 0.25), 0.6)
}

// alert — короткая сирена (две «волны»): перед голосовым оповещением «Мид, ганк!».
func alert() Clip {
	c := buf(1.25)
	ph := 0.0
	for i := range c {
		t := float64(i) / sr
		f := 700 + 800*(0.5-0.5*math.Cos(2*math.Pi*t/0.6))
		ph += 2 * math.Pi * f / sr
		v := math.Sin(ph) + 0.35*math.Sin(2*ph) + 0.15*math.Sin(3*ph)
		c[i] = float32(math.Tanh(v*1.8) * math.Min(1, t/0.02))
	}
	filt(c, "highpass", 300, 0.7, 0)
	return normalize(fadeOut(c, 0.12), 0.85)
}

// beep2 — «пик-пик» для привлечения внимания перед фразой.
func beep2() Clip {
	c := buf(0.36)
	for _, at := range []float64{0, 0.16} {
		addAt(c, tone(1320, 0.1, 0.6), at, 1)
		addAt(c, tone(2640, 0.1, 0.12), at, 1)
	}
	return normalize(c, 0.7)
}
