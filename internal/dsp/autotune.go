package dsp

import "math"

// AutoTune — подтяжка высоты голоса к ближайшей ноте («автотюн»). Высота голоса ищется алгоритмом
// YIN на сигнале, прореженном до ~12 кГц; сдвиг — «вращающейся головкой» с окном, кратным периоду
// голоса (меньше металлической окраски на стыках). Задержка ~15–25 мс.
type AutoTune struct {
	sr    float64
	key   int // 0 — до (C) … 11 — си (B)
	notes []bool
	shift float64 // постоянный сдвиг, полутоны
	mix   float32
	coef  float64 // сглаживание поправки (1 — мгновенно)

	aa1, aa2   *Biquad // антиалиасинг перед прореживанием
	decF       int
	dsr        float64
	dec        [atDecN]float32
	decW       int
	decPh      int
	hop        int
	tMin, tMax int
	x, d       []float64

	note     float64 // нота (MIDI), к которой тянем; 0 — голоса нет
	tgt, cur float64 // поправка, полутоны
	quiet    int     // отсчётов без голоса подряд

	dl          *dline
	phase       float64
	win, winTgt float64
}

const (
	atDecN = 1024
	atW    = 360 // окно анализа: 30 мс на 12 кГц
	atHop  = 384 // анализ каждые 8 мс
)

// Лады: какие ступени от тоники разрешены.
var scaleTab = map[string][]bool{
	"chromatic": {true, true, true, true, true, true, true, true, true, true, true, true},
	"major":     {true, false, true, false, true, true, false, true, false, true, false, true},
	"minor":     {true, false, true, true, false, true, false, true, true, false, true, false},
}

// NewAutoTune: key 0..11 (до…си), scale chromatic|major|minor, speed 0..1 (0 — мгновенно, «робот»;
// 1 — плавно, естественно), shift — сдвиг в полутонах, mix 0..1.
func NewAutoTune(key int, scale string, speed, shift, mix, sr float64) *AutoTune {
	notes, ok := scaleTab[scale]
	if !ok {
		notes = scaleTab["chromatic"]
	}
	a := &AutoTune{sr: sr, key: ((key % 12) + 12) % 12, notes: notes, shift: shift, mix: float32(max(0, min(1, mix)))}
	speed = max(0, min(1, speed))
	if tau := speed * speed * 0.25; tau < 1e-4 {
		a.coef = 1
	} else {
		a.coef = 1 - math.Exp(-1/(tau*sr))
	}
	a.decF = max(1, int(math.Round(sr/12000)))
	a.dsr = sr / float64(a.decF)
	a.tMin = int(a.dsr / 1000)
	a.tMax = int(a.dsr / 70)
	if atW+a.tMax+1 > atDecN {
		a.tMax = atDecN - atW - 1
	}
	a.aa1 = NewBiquad("lowpass", 0.23*a.dsr, 0.54, 0, sr)
	a.aa2 = NewBiquad("lowpass", 0.23*a.dsr, 1.31, 0, sr)
	a.x = make([]float64, atW+a.tMax+1)
	a.d = make([]float64, a.tMax+2)
	a.win, a.winTgt = 0.03*sr, 0.03*sr
	a.dl = newDline(int(0.06*sr) + 8)
	a.tgt, a.cur = shift, shift
	return a
}

func (a *AutoTune) Process(b []float32) {
	for i, x := range b {
		v := a.aa2.tick(a.aa1.tick(float64(x)))
		if a.decPh++; a.decPh >= a.decF {
			a.decPh = 0
			a.dec[a.decW&(atDecN-1)] = float32(v)
			a.decW++
		}
		if a.hop++; a.hop >= atHop {
			a.hop = 0
			a.analyze()
		}
		a.cur += (a.tgt - a.cur) * a.coef
		a.win += (a.winTgt - a.win) * 0.0005
		ratio := math.Exp2(a.cur / 12)
		a.dl.push(x)
		a.phase += (1 - ratio) / a.win
		a.phase -= math.Floor(a.phase)
		ph2 := a.phase + 0.5
		if ph2 >= 1 {
			ph2--
		}
		s := math.Sin(math.Pi * a.phase)
		g1 := float32(s * s)
		wet := g1*a.dl.tap(1+a.phase*a.win) + (1-g1)*a.dl.tap(1+ph2*a.win)
		b[i] = x*(1-a.mix) + wet*a.mix
	}
}

// analyze — YIN по последним 30 мс: частота основного тона или «голоса нет».
func (a *AutoTune) analyze() {
	n := atW + a.tMax
	if a.decW < n {
		return
	}
	start := a.decW - n
	en := 0.0
	for j := 0; j < n; j++ {
		a.x[j] = float64(a.dec[(start+j)&(atDecN-1)])
	}
	ref := a.x[a.tMax:] // самые свежие atW отсчётов
	for j := 0; j < atW; j++ {
		en += ref[j] * ref[j]
	}
	if en/atW < 3e-6 { // тише ~−55 дБ — голоса нет
		a.unvoiced()
		return
	}
	sum := 0.0
	for tau := 1; tau <= a.tMax; tau++ {
		s := 0.0
		lag := a.x[a.tMax-tau:]
		for j := 0; j < atW; j++ {
			df := ref[j] - lag[j]
			s += df * df
		}
		sum += s
		if sum > 0 {
			a.d[tau] = s * float64(tau) / sum
		} else {
			a.d[tau] = 1
		}
	}
	tau := -1
	for t := a.tMin; t <= a.tMax; t++ {
		if a.d[t] < 0.15 {
			for t+1 <= a.tMax && a.d[t+1] < a.d[t] {
				t++
			}
			tau = t
			break
		}
	}
	if tau < 0 {
		mn := math.Inf(1)
		for t := a.tMin; t <= a.tMax; t++ {
			if a.d[t] < mn {
				mn, tau = a.d[t], t
			}
		}
		if mn > 0.3 {
			a.unvoiced()
			return
		}
	}
	ft := float64(tau)
	if tau > 1 && tau < a.tMax {
		s0, s1, s2 := a.d[tau-1], a.d[tau], a.d[tau+1]
		if den := s0 + s2 - 2*s1; den > 0 {
			ft += 0.5 * (s0 - s2) / den
		}
	}
	a.voiced(a.dsr / ft)
}

func (a *AutoTune) voiced(f0 float64) {
	m := 69 + 12*math.Log2(f0/440)
	a.note = a.nearest(m)
	a.tgt = a.note - m + a.shift
	a.quiet = 0
	// половина окна — целое число периодов (не меньше 12 мс): головки на стыке звучат в фазе
	T := a.sr / f0
	k := math.Ceil(0.012 * a.sr / T)
	a.winTgt = max(0.02*a.sr, min(0.05*a.sr, 2*k*T))
}

func (a *AutoTune) unvoiced() {
	a.quiet += atHop
	if a.quiet > int(0.15*a.sr) { // пауза — отпускаем к простому сдвигу
		a.tgt, a.note = a.shift, 0
	}
}

// nearest — ближайшая разрешённая нота; текущую держим, пока другая не ближе заметно (без дрожания).
func (a *AutoTune) nearest(m float64) float64 {
	ok := func(n int) bool { return a.notes[((n-a.key)%12+12)%12] }
	r := int(math.Round(m))
	best, bd := float64(r), math.Inf(1)
	for n := r - 3; n <= r+3; n++ {
		if d := math.Abs(float64(n) - m); ok(n) && d < bd {
			best, bd = float64(n), d
		}
	}
	if a.note != 0 && ok(int(a.note)) && math.Abs(m-a.note) < bd+0.15 {
		return a.note
	}
	return best
}
