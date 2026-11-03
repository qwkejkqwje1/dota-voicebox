package dsp

import "math"

// Фоновые звуки «под голос»: дождь, гроза, ветер, костёр, море, космос, пластинка. Всё синтезируется
// на лету (без файлов и без повторов), громкость приведена к одному уровню (~−20 дБ).

// Ambience — бесконечный генератор фона; Render перезаписывает буфер.
type Ambience interface {
	Render(dst []float32)
}

// AmbienceInfo — фон для интерфейса.
type AmbienceInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Emoji string `json:"emoji"`
	Desc  string `json:"desc"`
}

// Ambiences — встроенные фоны по порядку.
var Ambiences = []AmbienceInfo{
	{ID: "rain", Title: "Дождь", Emoji: "🌧️", Desc: "Ровный дождь за окном"},
	{ID: "storm", Title: "Гроза", Emoji: "⛈️", Desc: "Ливень и далёкие раскаты грома"},
	{ID: "wind", Title: "Ветер", Emoji: "🌬️", Desc: "Порывы ветра в горах"},
	{ID: "fire", Title: "Костёр", Emoji: "🔥", Desc: "Треск поленьев у огня"},
	{ID: "sea", Title: "Море", Emoji: "🌊", Desc: "Волны набегают на берег"},
	{ID: "space", Title: "Космос", Emoji: "🚀", Desc: "Гул космического корабля и сигналы приборов"},
	{ID: "vinyl", Title: "Пластинка", Emoji: "📀", Desc: "Шорох и потрескивание винила"},
}

// NewAmbience создаёт фон по id (nil — нет такого).
func NewAmbience(id string, sr float64) Ambience {
	var g ambGen
	switch id {
	case "rain":
		g = newRain(sr, false)
	case "storm":
		g = newRain(sr, true)
	case "wind":
		g = newWind(sr)
	case "fire":
		g = newFire(sr)
	case "sea":
		g = newSea(sr)
	case "space":
		g = newSpace(sr)
	case "vinyl":
		g = newVinyl(sr)
	default:
		return nil
	}
	return newLeveled(g, sr)
}

type ambGen interface{ next() float64 }

// leveled приводит громкость генератора к ~0,1 RMS (замер по первым двум секундам).
type leveled struct {
	g    ambGen
	hp   *Biquad // срез инфранизких (их всё равно режут голосовые чаты)
	gain float64
}

func newLeveled(g ambGen, sr float64) *leveled {
	l := &leveled{g: g, hp: NewBiquad("highpass", 35, 0.707, 0, sr)}
	n := int(2 * sr)
	e := 0.0
	for i := 0; i < n; i++ {
		x := l.hp.tick(g.next())
		e += x * x
	}
	rms := math.Sqrt(e / float64(n))
	l.gain = 1
	if rms > 1e-6 {
		l.gain = 0.1 / rms
	}
	return l
}

func (l *leveled) Render(dst []float32) {
	for i := range dst {
		dst[i] = float32(l.hp.tick(l.g.next()) * l.gain)
	}
}

// ---------- кирпичики ----------

// pinkN — розовый шум (фильтр Пола Келлета).
type pinkN struct{ b0, b1, b2, b3, b4, b5, b6 float64 }

func (p *pinkN) tick(w float64) float64 {
	p.b0 = 0.99886*p.b0 + w*0.0555179
	p.b1 = 0.99332*p.b1 + w*0.0750759
	p.b2 = 0.96900*p.b2 + w*0.1538520
	p.b3 = 0.86650*p.b3 + w*0.3104856
	p.b4 = 0.55000*p.b4 + w*0.5329522
	p.b5 = -0.7616*p.b5 - w*0.0168980
	out := p.b0 + p.b1 + p.b2 + p.b3 + p.b4 + p.b5 + p.b6 + w*0.5362
	p.b6 = w * 0.115926
	return out * 0.11
}

// brownN — «коричневый» (красный) шум: гул.
type brownN struct{ v float64 }

func (b *brownN) tick(w float64) float64 {
	b.v = (b.v + 0.02*w) / 1.02
	return b.v * 3.5
}

// onePole — простейший фильтр нижних частот.
type onePole struct{ y, a float64 }

func newOnePole(fc, sr float64) onePole { return onePole{a: 1 - math.Exp(-2*math.Pi*fc/sr)} }

func (o *onePole) tick(x float64) float64 {
	o.y += (x - o.y) * o.a
	return o.y
}

// drift — медленно блуждающая величина 0..1 (порывы ветра, яркость огня).
type drift struct {
	v1, v2, tgt float64
	rate        float64
	cnt         int
	minN, maxN  int
}

func newDrift(minSec, maxSec, smoothSec, sr float64) drift {
	return drift{minN: int(minSec * sr), maxN: int(maxSec * sr), rate: 1 - math.Exp(-1/(smoothSec*sr)), v1: 0.5, v2: 0.5, tgt: 0.5}
}

func (d *drift) tick(r *rng) float64 {
	if d.cnt--; d.cnt <= 0 {
		d.tgt = r.unit()
		d.cnt = d.minN + int(r.unit()*float64(d.maxN-d.minN))
	}
	d.v1 += (d.tgt - d.v1) * d.rate
	d.v2 += (d.v1 - d.v2) * d.rate
	return d.v2
}

// svf — фильтр с переменной частотой (TPT state variable filter).
type svf struct{ ic1, ic2, g, k float64 }

func (s *svf) set(fc, q, sr float64) {
	s.g = math.Tan(math.Pi * min(fc, 0.45*sr) / sr)
	s.k = 1 / q
}

func (s *svf) tick(x float64) (lp, bp float64) {
	a1 := 1 / (1 + s.g*(s.g+s.k))
	a2 := s.g * a1
	a3 := s.g * a2
	v3 := x - s.ic2
	v1 := a1*s.ic1 + a2*v3
	v2 := s.ic2 + a2*s.ic1 + a3*v3
	s.ic1 = 2*v1 - s.ic1
	s.ic2 = 2*v2 - s.ic2
	return v2, v1
}

// reso — затухающий резонатор (капля, щелчок с тоном).
type reso struct{ y1, y2, c, r2 float64 }

func (o *reso) hit(freq, decaySec, amp, sr float64) {
	r := math.Exp(-1 / (decaySec * sr))
	w := 2 * math.Pi * freq / sr
	o.c, o.r2 = 2*r*math.Cos(w), r*r
	o.y1, o.y2 = amp*math.Sin(w), 0
}

func (o *reso) tick() float64 {
	y := o.c*o.y1 - o.r2*o.y2
	o.y2, o.y1 = o.y1, y
	return y
}

func (r *rng) w() float64 { return float64(r.bi()) }

// ---------- дождь и гроза ----------

type rain struct {
	sr        float64
	r         rng
	pk        pinkN
	hp1, hp2  *Biquad
	lp        *Biquad
	patHP     *Biquad
	patEnv    float64
	patDec    float64
	drops     [12]reso
	dropI     int
	rumble    brownN
	rumbleLP  onePole
	intensity drift
	storm     bool
	th        thunder
}

func newRain(sr float64, storm bool) *rain {
	g := &rain{sr: sr, r: rng{s: 0x2545F491}, storm: storm,
		hp1: NewBiquad("highpass", 450, 0.707, 0, sr), hp2: NewBiquad("highpass", 450, 0.707, 0, sr),
		lp: NewBiquad("lowpass", 9000, 0.707, 0, sr), patHP: NewBiquad("highpass", 2200, 0.707, 0, sr),
		rumbleLP: newOnePole(180, sr), intensity: newDrift(3, 8, 1.5, sr)}
	if storm {
		g.th = newThunder(sr)
	}
	return g
}

func (g *rain) next() float64 {
	in := 0.75 + 0.25*g.intensity.tick(&g.r)
	if g.storm {
		in *= 1.35
	}
	hiss := g.lp.tick(g.hp2.tick(g.hp1.tick(g.pk.tick(g.r.w())))) * 0.9 * in
	// мелкая дробь капель: короткие вспышки шума
	if g.r.unit() < 400*in/g.sr {
		a := g.r.unit()
		g.patEnv = math.Max(g.patEnv, 0.15+0.85*a*a*a)
		g.patDec = math.Exp(-1 / ((0.0004 + 0.0012*g.r.unit()) * g.sr))
	}
	pat := g.patHP.tick(g.r.w()*g.patEnv) * 0.9
	g.patEnv *= g.patDec
	// крупные капли с тоном
	if g.r.unit() < 9*in/g.sr {
		g.drops[g.dropI].hit(1200+3500*g.r.unit(), 0.004+0.012*g.r.unit(), 0.05+0.25*g.r.unit(), g.sr)
		g.dropI = (g.dropI + 1) % len(g.drops)
	}
	d := 0.0
	for i := range g.drops {
		d += g.drops[i].tick()
	}
	low := g.rumbleLP.tick(g.rumble.tick(g.r.w())) * 0.5
	out := hiss + pat + d + low
	if g.storm {
		out += g.th.tick(&g.r)
	}
	return out
}

// thunder — раскаты грома: раз в 12–35 с, с треском в начале у близких.
type thunder struct {
	sr           float64
	wait, t, dur int
	amp, crack   float64
	br           brownN
	lp1, lp2     onePole
	crackHP      *Biquad
	roll         drift
}

func newThunder(sr float64) thunder {
	return thunder{sr: sr, wait: int(6 * sr), lp1: newOnePole(380, sr), lp2: newOnePole(380, sr),
		crackHP: NewBiquad("highpass", 900, 0.707, 0, sr), roll: newDrift(0.08, 0.5, 0.05, sr)}
}

func (th *thunder) tick(r *rng) float64 {
	if th.dur == 0 {
		if th.wait--; th.wait > 0 {
			return 0
		}
		th.t, th.dur = 0, int((3+3*r.unit())*th.sr)
		th.amp = 1.2 + 1.8*r.unit()
		th.crack = 0
		if r.unit() < 0.45 {
			th.crack = 0.4 + 0.6*r.unit()
		}
	}
	t := float64(th.t) / th.sr
	att := math.Min(1, t/0.18)
	env := att * att * math.Exp(-3.2*t/(float64(th.dur)/th.sr))
	rumble := th.lp2.tick(th.lp1.tick(th.br.tick(r.w()))) * env * th.amp * (0.35 + 0.65*th.roll.tick(r))
	if th.crack > 0 && t < 0.4 {
		rumble += th.crackHP.tick(r.w()) * th.crack * math.Exp(-t/0.07) * 0.6
	}
	if th.t++; th.t >= th.dur {
		th.dur, th.wait = 0, int((12+23*r.unit())*th.sr)
	}
	return rumble * 3
}

// ---------- ветер ----------

type wind struct {
	sr       float64
	r        rng
	gust     drift
	whistle  drift
	core, wh svf
	br       brownN
	lowLP    onePole
	cnt      int
	gv, wv   float64
}

func newWind(sr float64) *wind {
	return &wind{sr: sr, r: rng{s: 0x9E3779B1}, gust: newDrift(1.5, 5, 0.6, sr), whistle: newDrift(2, 6, 0.8, sr), lowLP: newOnePole(120, sr)}
}

func (g *wind) next() float64 {
	g.gv = g.gust.tick(&g.r)
	g.wv = g.whistle.tick(&g.r)
	if g.cnt--; g.cnt <= 0 {
		g.cnt = 16
		g.core.set(160+750*g.gv, 1.1+1.8*g.gv, g.sr)
		g.wh.set(850+1000*g.wv, 9, g.sr)
	}
	w := g.r.w()
	_, c := g.core.tick(w)
	_, h := g.wh.tick(g.r.w())
	low := g.lowLP.tick(g.br.tick(g.r.w()))
	gv := g.gv * g.gv
	return c*(0.25+0.9*gv) + h*0.35*g.wv*g.wv*gv + low*(0.25+0.4*g.gv)
}

// ---------- костёр ----------

type fire struct {
	sr          float64
	r           rng
	br          brownN
	roarLP      onePole
	flicker     drift
	burst       drift
	hissHP      *Biquad
	clickHP     *Biquad
	popLP       *Biquad
	clickEnv    float64
	clickDec    float64
	popEnv      float64
	popDec      float64
	cluster     int // ещё щелчков в серии
	clusterWait int
}

func newFire(sr float64) *fire {
	return &fire{sr: sr, r: rng{s: 0x85EBCA6B}, roarLP: newOnePole(380, sr), flicker: newDrift(0.15, 0.7, 0.08, sr),
		burst: newDrift(1, 3, 0.4, sr), hissHP: NewBiquad("highpass", 3000, 0.707, 0, sr),
		clickHP: NewBiquad("highpass", 1400, 0.707, 0, sr), popLP: NewBiquad("lowpass", 1800, 0.707, 0, sr)}
}

func (g *fire) next() float64 {
	fl := 0.55 + 0.45*g.flicker.tick(&g.r)
	roar := g.roarLP.tick(g.br.tick(g.r.w())) * 1.1 * fl
	hiss := g.hissHP.tick(g.r.w()) * 0.05 * fl
	b := g.burst.tick(&g.r)
	if g.cluster == 0 && g.r.unit() < (2+14*b*b)/g.sr {
		g.cluster = 1 + int(g.r.unit()*4)
		g.clusterWait = 0
		if g.r.unit() < 0.12 { // крупный «выстрел» полена
			g.popEnv = 0.6 + 0.4*g.r.unit()
			g.popDec = math.Exp(-1 / ((0.006 + 0.012*g.r.unit()) * g.sr))
		}
	}
	if g.cluster > 0 {
		if g.clusterWait--; g.clusterWait <= 0 {
			a := g.r.unit()
			g.clickEnv = math.Max(g.clickEnv, 0.2+0.8*a*a)
			g.clickDec = math.Exp(-1 / ((0.0003 + 0.0018*g.r.unit()) * g.sr))
			g.cluster--
			g.clusterWait = int((0.004 + 0.03*g.r.unit()) * g.sr)
		}
	}
	click := g.clickHP.tick(g.r.w()*g.clickEnv) * 1.6
	g.clickEnv *= g.clickDec
	pop := g.popLP.tick(g.r.w()*g.popEnv) * 1.2
	g.popEnv *= g.popDec
	return roar + hiss + click + pop
}

// ---------- море ----------

type wave struct {
	t, period, amp float64
	pk             pinkN
	f              svf
	cnt            int
}

type sea struct {
	sr     float64
	r      rng
	waves  [2]wave
	br     brownN
	washLP onePole
}

func newSea(sr float64) *sea {
	g := &sea{sr: sr, r: rng{s: 0xC2B2AE35}, washLP: newOnePole(300, sr)}
	for i := range g.waves {
		g.newWave(&g.waves[i])
		g.waves[i].t = float64(i) * 0.5 * g.waves[i].period
	}
	return g
}

func (g *sea) newWave(w *wave) {
	w.t, w.period, w.amp = 0, (7+5*g.r.unit())*g.sr, 0.6+0.4*g.r.unit()
}

// env — форма волны: медленный набег (65%), потом откат.
func (w *wave) env() float64 {
	x := w.t / w.period
	if x < 0.65 {
		s := math.Sin(math.Pi / 2 * x / 0.65)
		return s * s
	}
	return math.Exp(-4 * (x - 0.65) / 0.35)
}

func (g *sea) next() float64 {
	out := g.washLP.tick(g.br.tick(g.r.w())) * 0.25
	for i := range g.waves {
		w := &g.waves[i]
		e := w.env() * w.amp
		if w.cnt--; w.cnt <= 0 {
			w.cnt = 32
			w.f.set(250+3200*math.Pow(e, 1.5), 0.7, g.sr)
		}
		lp, _ := w.f.tick(w.pk.tick(g.r.w()))
		out += lp * (0.12 + 0.88*e)
		if w.t++; w.t >= w.period {
			g.newWave(w)
		}
	}
	return out
}

// ---------- космос: гул корабля, вентиляция, приборы ----------

type space struct {
	sr        float64
	r         rng
	ph        [4]float64
	pk        pinkN
	ventLP    onePole
	ventLP2   onePole
	sweep     svf
	sweepPh   float64
	cnt       int
	beat      float64
	beepWait  int
	beepT     int
	beepTones [2]float64
	beepPh    float64
}

func newSpace(sr float64) *space {
	return &space{sr: sr, r: rng{s: 0x27D4EB2F}, ventLP: newOnePole(650, sr), ventLP2: newOnePole(650, sr), beepWait: int(5 * sr), beepT: -1}
}

func (g *space) next() float64 {
	freqs := [4]float64{55, 110.35, 165.2, 55.25}
	amps := [4]float64{0.5, 0.22, 0.08, 0.35}
	hum := 0.0
	for i := range g.ph {
		g.ph[i] += 2 * math.Pi * freqs[i] / g.sr
		if g.ph[i] > 2*math.Pi {
			g.ph[i] -= 2 * math.Pi
		}
		hum += math.Sin(g.ph[i]) * amps[i]
	}
	vent := g.ventLP2.tick(g.ventLP.tick(g.pk.tick(g.r.w()))) * 2.2
	g.sweepPh += 2 * math.Pi * 0.045 / g.sr
	if g.cnt--; g.cnt <= 0 {
		g.cnt = 32
		g.sweep.set(380+700*(0.5+0.5*math.Sin(g.sweepPh)), 5, g.sr)
	}
	_, sw := g.sweep.tick(g.r.w())
	out := hum*0.35 + vent + sw*0.06
	// тихие сигналы приборов
	if g.beepT < 0 {
		if g.beepWait--; g.beepWait <= 0 {
			g.beepT = 0
			base := []float64{1318.5, 1568, 1760, 2093}[int(g.r.unit()*4)%4]
			g.beepTones = [2]float64{base, base * 1.335}
		}
	} else {
		n := int(0.075 * g.sr)
		k := g.beepT / n
		if k >= 2 {
			g.beepT, g.beepWait = -1, int((7+12*g.r.unit())*g.sr)
		} else {
			x := float64(g.beepT%n) / float64(n)
			env := math.Sin(math.Pi * x)
			g.beepPh += 2 * math.Pi * g.beepTones[k] / g.sr
			out += math.Sin(g.beepPh) * env * 0.08
			g.beepT++
		}
	}
	return out
}

// ---------- пластинка ----------

type vinyl struct {
	sr       float64
	r        rng
	pk       pinkN
	bp1, bp2 *Biquad
	clickHP  *Biquad
	popLP    *Biquad
	click    float64
	clickDec float64
	pop      float64
	popDec   float64
}

func newVinyl(sr float64) *vinyl {
	return &vinyl{sr: sr, r: rng{s: 0x165667B1}, bp1: NewBiquad("highpass", 250, 0.707, 0, sr), bp2: NewBiquad("lowpass", 5000, 0.707, 0, sr),
		clickHP: NewBiquad("highpass", 1800, 0.707, 0, sr), popLP: NewBiquad("lowpass", 1200, 0.707, 0, sr)}
}

func (g *vinyl) next() float64 {
	surf := g.bp2.tick(g.bp1.tick(g.pk.tick(g.r.w()))) * 0.35
	if g.r.unit() < 30/g.sr {
		a := g.r.unit()
		g.click = math.Max(g.click, 0.1+0.9*a*a*a)
		g.clickDec = math.Exp(-1 / ((0.0001 + 0.0004*g.r.unit()) * g.sr))
	}
	if g.r.unit() < 0.6/g.sr {
		g.pop = 0.5 + 0.5*g.r.unit()
		g.popDec = math.Exp(-1 / (0.004 * g.sr))
	}
	c := g.clickHP.tick(g.r.w()*g.click) * 0.55
	p := g.popLP.tick(g.r.w()*g.pop) * 0.5
	g.click *= g.clickDec
	g.pop *= g.popDec
	return surf + c + p
}

// ---------- свой звук по кругу ----------

// Loop — свой звук фоном по кругу, со стыком без щелчка (перекрытие 50 мс).
type Loop struct {
	data []float32
	pos  int
	xf   int
}

// NewLoop — nil, если звук слишком короткий.
func NewLoop(data []float32, sr float64) *Loop {
	xf := int(0.05 * sr)
	if len(data) < 4*xf {
		return nil
	}
	return &Loop{data: data, xf: xf}
}

func (l *Loop) Render(dst []float32) {
	n := len(l.data)
	for i := range dst {
		y := l.data[l.pos]
		if k := l.pos - (n - l.xf); k >= 0 { // хвост перетекает в начало
			t := float32(k) / float32(l.xf)
			y = y*(1-t) + l.data[k]*t
		}
		dst[i] = y
		if l.pos++; l.pos >= n {
			l.pos = l.xf
		}
	}
}
