package sounds

import (
	"math"
	"math/rand"
	"sort"
)

// Встроенные звуки синтезируются в коде — не нужны файлы в репозитории.
// В конфиге на них ссылаются как "builtin:<имя>".
var builtins = map[string]func() Clip{
	"siren":       siren,      // сирена «мид ганкают»
	"wisdom_rune": wisdomRune, // руна мудрости (экспа)
	"mid_rune_6":  midRune6,   // первая руна силы на 6:00
	"power_rune":  powerRune,  // руны силы каждые 2 минуты
	"bounty_rune": bountyRune, // руны богатства
	"rosh":        rosh,       // Рошан / аегис
	"stack":       stack,      // тик для стака крипов
	"beep":        func() Clip { return tone(1000, 0.15, 0.5) },
	"danger":      danger, // короткий тревожный сигнал
}

// BuiltinNames — список для справки.
func BuiltinNames() []string {
	var n []string
	for k := range builtins {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

func Builtin(name string) (Clip, bool) {
	f, ok := builtins[name]
	if !ok {
		return nil, false
	}
	return f(), true
}

const sr = float64(SampleRate)

func buf(sec float64) Clip { return make(Clip, int(sec*sr)) }

func tone(f, sec, amp float64) Clip {
	c := buf(sec)
	for i := range c {
		t := float64(i) / sr
		env := math.Min(1, t/0.005) * math.Min(1, (sec-t)/0.02)
		c[i] = float32(amp * env * math.Sin(2*math.Pi*f*t))
	}
	return c
}

// bell — колокольчик: основной тон + негармонические обертоны с затуханием.
func bell(c Clip, at, f, amp, decay float64) {
	start := int(at * sr)
	partials := []struct{ r, a, d float64 }{{1, 1, 1}, {2.0, 0.5, 0.7}, {2.76, 0.35, 0.5}, {5.4, 0.15, 0.3}}
	for i := start; i < len(c); i++ {
		t := float64(i-start) / sr
		var v float64
		for _, p := range partials {
			v += p.a * math.Exp(-t/(decay*p.d)) * math.Sin(2*math.Pi*f*p.r*t)
		}
		v *= math.Min(1, t/0.002)
		c[i] += float32(amp * v)
	}
}

func normalize(c Clip, peak float32) Clip {
	var m float32
	for _, x := range c {
		if x < 0 {
			x = -x
		}
		if x > m {
			m = x
		}
	}
	if m > 0 {
		k := peak / m
		for i := range c {
			c[i] *= k
		}
	}
	return c
}

func fadeOut(c Clip, sec float64) Clip {
	n := int(sec * sr)
	for i := 0; i < n && i < len(c); i++ {
		c[len(c)-1-i] *= float32(i) / float32(n)
	}
	return c
}

// siren — воздушная тревога: плавный подъём/спад 600↔1500 Гц, 3 цикла.
func siren() Clip {
	c := buf(3.0)
	ph := 0.0
	for i := range c {
		t := float64(i) / sr
		f := 600 + 900*(0.5-0.5*math.Cos(2*math.Pi*t/1.0))
		ph += 2 * math.Pi * f / sr
		v := math.Sin(ph) + 0.35*math.Sin(2*ph) + 0.15*math.Sin(3*ph)
		v = math.Tanh(v * 1.8)
		c[i] = float32(v * math.Min(1, t/0.05))
	}
	return normalize(fadeOut(c, 0.15), 0.9)
}

// wisdomRune — «магическое» восходящее арпеджио (руна опыта).
func wisdomRune() Clip {
	c := buf(1.8)
	notes := []float64{659.25, 830.61, 987.77, 1318.5, 1661.2} // E5 G#5 B5 E6 G#6
	for i, f := range notes {
		bell(c, float64(i)*0.09, f, 0.5, 0.45)
	}
	bell(c, 0.55, 1318.5, 0.35, 0.8)
	bell(c, 0.55, 987.77, 0.3, 0.8)
	return normalize(fadeOut(c, 0.3), 0.85)
}

// midRune6 — фанфара: первая руна силы в миде на 6:00.
func midRune6() Clip {
	c := buf(1.6)
	seq := []struct{ at, f, d float64 }{
		{0, 523.25, 0.12}, {0.12, 659.25, 0.12}, {0.24, 783.99, 0.12}, {0.36, 1046.5, 0.9},
	}
	for _, n := range seq {
		s := int(n.at * sr)
		for i := s; i < len(c) && i < s+int((n.d+0.3)*sr); i++ {
			t := float64(i-s) / sr
			env := math.Min(1, t/0.01) * math.Exp(-math.Max(0, t-n.d)/0.08)
			// «медный» тон: пилообразная с ограничением гармоник
			var v float64
			for h := 1.0; h <= 8; h++ {
				v += math.Sin(2*math.Pi*n.f*h*t) / h
			}
			c[i] += float32(0.3 * env * v)
		}
	}
	return normalize(fadeOut(c, 0.2), 0.85)
}

// powerRune — «динь-дон».
func powerRune() Clip {
	c := buf(1.3)
	bell(c, 0, 987.77, 0.6, 0.35)
	bell(c, 0.25, 739.99, 0.6, 0.5)
	return normalize(fadeOut(c, 0.2), 0.8)
}

// bountyRune — «монетки».
func bountyRune() Clip {
	c := buf(0.9)
	for i, at := range []float64{0, 0.08, 0.16} {
		bell(c, at, 1975.5+float64(i)*200, 0.4, 0.15)
	}
	return normalize(fadeOut(c, 0.15), 0.75)
}

// rosh — низкий рёв/горн.
func rosh() Clip {
	c := buf(1.4)
	ph := 0.0
	for i := range c {
		t := float64(i) / sr
		f := 82 + 6*math.Sin(2*math.Pi*5*t)
		ph += 2 * math.Pi * f / sr
		var v float64
		for h := 1.0; h <= 12; h++ {
			v += math.Sin(h*ph) / h
		}
		v += (rand.Float64()*2 - 1) * 0.15
		env := math.Min(1, t/0.08) * math.Min(1, (1.4-t)/0.3)
		c[i] = float32(math.Tanh(v*2) * env)
	}
	return normalize(c, 0.85)
}

func stack() Clip {
	c := buf(0.5)
	bell(c, 0, 1500, 0.5, 0.08)
	bell(c, 0.15, 1500, 0.5, 0.08)
	return normalize(c, 0.6)
}

func danger() Clip {
	c := buf(0.9)
	for k := 0; k < 3; k++ {
		t := tone(1760, 0.12, 0.7)
		copy(c[int(float64(k)*0.25*sr):], t)
	}
	return normalize(c, 0.8)
}
