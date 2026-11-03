package dsp

import "math"

// ---------- общие помощники ----------

// rng — быстрый генератор шума для аудиопотока (без блокировок math/rand).
type rng struct{ s uint32 }

func (r *rng) next() uint32 {
	x := r.s
	if x == 0 {
		x = 0x9E3779B9
	}
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	r.s = x
	return x
}

// bi — равномерный шум [-1, 1).
func (r *rng) bi() float32 { return float32(r.next()>>8)/float32(1<<23) - 1 }

// unit — [0, 1).
func (r *rng) unit() float64 { return float64(r.next()>>8) / float64(1<<24) }

func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// dline — линия задержки с дробным чтением.
type dline struct {
	buf  []float32
	mask int
	w    int
}

func newDline(maxDelay int) *dline {
	n := nextPow2(maxDelay + 4)
	return &dline{buf: make([]float32, n), mask: n - 1}
}

func (d *dline) push(x float32) {
	d.buf[d.w] = x
	d.w = (d.w + 1) & d.mask
}

// tap читает отсчёт с задержкой delay (в отсчётах, >= 1) с линейной интерполяцией.
func (d *dline) tap(delay float64) float32 {
	i := int(delay)
	f := float32(delay - float64(i))
	a := d.buf[(d.w-i)&d.mask]
	b := d.buf[(d.w-i-1)&d.mask]
	return a + (b-a)*f
}

func flush(x float32) float32 {
	if x > -1e-15 && x < 1e-15 {
		return 0
	}
	return x
}

// ---------- сдвиг высоты тона ----------

// PitchShift — сдвиг высоты тона «вращающейся головкой»: две точки чтения
// линии задержки с плавной перекрёстной сменой (как в педалях-гармонайзерах).
// Задержка в среднем — половина окна (~20 мс).
type PitchShift struct {
	ratio, win, phase float64
	mix               float32
	d                 *dline
}

func NewPitchShift(semitones, windowMS, mix, sr float64) *PitchShift {
	win := windowMS * sr / 1000
	return &PitchShift{ratio: math.Pow(2, semitones/12), win: win, mix: float32(mix), d: newDline(int(win) + 8)}
}

func (p *PitchShift) Process(b []float32) {
	step := (1 - p.ratio) / p.win
	for i, x := range b {
		p.d.push(x)
		p.phase += step
		p.phase -= math.Floor(p.phase)
		ph2 := p.phase + 0.5
		if ph2 >= 1 {
			ph2--
		}
		s := math.Sin(math.Pi * p.phase)
		g1 := float32(s * s)
		wet := g1*p.d.tap(1+p.phase*p.win) + (1-g1)*p.d.tap(1+ph2*p.win)
		b[i] = x*(1-p.mix) + wet*p.mix
	}
}

// ---------- реверберация (Freeverb) ----------

type combF struct {
	buf        []float32
	i          int
	fb, d1, d2 float32
	store      float32
}

func (c *combF) process(x float32) float32 {
	y := c.buf[c.i]
	c.store = flush(y*c.d2 + c.store*c.d1)
	c.buf[c.i] = x + c.store*c.fb
	if c.i++; c.i >= len(c.buf) {
		c.i = 0
	}
	return y
}

type allpassF struct {
	buf []float32
	i   int
}

func (a *allpassF) process(x float32) float32 {
	bo := a.buf[a.i]
	a.buf[a.i] = flush(x + bo*0.5)
	if a.i++; a.i >= len(a.buf) {
		a.i = 0
	}
	return bo - x
}

// Reverb — классический Freeverb (8 гребенчатых + 4 всепропускающих фильтра), моно.
type Reverb struct {
	combs    []combF
	aps      []allpassF
	pre      *dline
	preN     float64
	wet, dry float32
}

func NewReverb(room, damp, wet, dry, preDelayMS, sr float64) *Reverb {
	scale := sr / 44100
	r := &Reverb{wet: float32(wet * 3), dry: float32(dry)}
	fb := float32(0.7 + 0.28*clamp01(room))
	d1 := float32(0.4 * clamp01(damp))
	for k, t := range []int{1116, 1188, 1277, 1356, 1422, 1491, 1557, 1617} {
		n := int(float64(t)*scale) + k // небольшое рассогласование
		r.combs = append(r.combs, combF{buf: make([]float32, n), fb: fb, d1: d1, d2: 1 - d1})
	}
	for _, t := range []int{556, 441, 341, 225} {
		r.aps = append(r.aps, allpassF{buf: make([]float32, int(float64(t)*scale))})
	}
	if preDelayMS > 0 {
		r.preN = preDelayMS * sr / 1000
		r.pre = newDline(int(r.preN) + 2)
	}
	return r
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

func (r *Reverb) Process(b []float32) {
	for i, x := range b {
		in := x
		if r.pre != nil {
			r.pre.push(x)
			in = r.pre.tap(r.preN)
		}
		in *= 0.015
		var acc float32
		for k := range r.combs {
			acc += r.combs[k].process(in)
		}
		for k := range r.aps {
			acc = r.aps[k].process(acc)
		}
		b[i] = x*r.dry + acc*r.wet
	}
}

// ---------- эхо ----------

// Echo — задержка с обратной связью и «тёмными» повторами (фильтр в петле).
type Echo struct {
	d           *dline
	n           float64
	fb, mix, lp float32
	state       float32
}

func NewEcho(delayMS, feedback, mix, toneHz, sr float64) *Echo {
	n := delayMS * sr / 1000
	return &Echo{d: newDline(int(n) + 2), n: n, fb: float32(math.Min(feedback, 0.95)), mix: float32(mix),
		lp: float32(1 - math.Exp(-2*math.Pi*toneHz/sr))}
}

func (e *Echo) Process(b []float32) {
	for i, x := range b {
		y := e.d.tap(e.n)
		e.state = flush(e.state + e.lp*(y-e.state))
		e.d.push(x + e.state*e.fb)
		b[i] = x + y*e.mix
	}
}

// ---------- модулированная задержка: хорус, флэнджер, вибрато ----------

type ModDelay struct {
	d                  *dline
	base, depth        float64
	inc, ph            float64
	fb, mix, dry, last float32
}

// NewModDelay: baseMS — минимальная задержка, depthMS — размах, dry<0 — без исходного сигнала (вибрато).
func NewModDelay(baseMS, depthMS, rateHz, feedback, mix, dry, sr float64) *ModDelay {
	base, depth := baseMS*sr/1000, depthMS*sr/1000
	m := &ModDelay{d: newDline(int(base+depth) + 4), base: math.Max(1, base), depth: depth,
		inc: 2 * math.Pi * rateHz / sr, fb: float32(math.Min(feedback, 0.9)), mix: float32(mix), dry: float32(dry)}
	if dry < 0 {
		m.dry = 0
	}
	return m
}

func (m *ModDelay) Process(b []float32) {
	for i, x := range b {
		m.ph += m.inc
		if m.ph > 2*math.Pi {
			m.ph -= 2 * math.Pi
		}
		y := m.d.tap(m.base + m.depth*(0.5+0.5*math.Sin(m.ph)))
		m.d.push(x + flush(y*m.fb))
		b[i] = x*m.dry + y*m.mix
	}
}

// Tremolo — пульсация громкости.
type Tremolo struct {
	Depth   float64
	inc, ph float64
}

func (t *Tremolo) Process(b []float32) {
	for i, x := range b {
		t.ph += t.inc
		if t.ph > 2*math.Pi {
			t.ph -= 2 * math.Pi
		}
		b[i] = x * float32(1-t.Depth*(0.5+0.5*math.Sin(t.ph)))
	}
}

// ---------- телефонный кодек и металл ----------

// MuLaw — 8-битный μ-law с частотой ~8 кГц (как в телефонной линии).
type MuLaw struct {
	Hold int
	cnt  int
	held float32
}

var muLog = math.Log1p(255)

func (m *MuLaw) Process(b []float32) {
	for i, x := range b {
		if m.cnt%m.Hold == 0 {
			v := math.Max(-1, math.Min(1, float64(x)))
			s := 1.0
			if v < 0 {
				s, v = -1, -v
			}
			y := math.Round(math.Log1p(255*v)/muLog*127) / 127
			m.held = float32(s * (math.Pow(256, y) - 1) / 255)
		}
		m.cnt++
		b[i] = m.held
	}
}

// Comb — резонатор с обратной связью: «металлический» призвук (робот, рупор).
type Comb struct {
	d            *dline
	n            float64
	fb, mix, nrm float32
}

func NewComb(freq, feedback, mix, sr float64) *Comb {
	n := sr / freq
	fb := math.Min(feedback, 0.95)
	return &Comb{d: newDline(int(n) + 2), n: n, fb: float32(fb), mix: float32(mix), nrm: float32(1 - fb*0.7)}
}

func (c *Comb) Process(b []float32) {
	for i, x := range b {
		y := x + c.fb*c.d.tap(c.n)
		c.d.push(flush(y))
		b[i] = x*(1-c.mix) + y*c.mix*c.nrm
	}
}
