// Package dsp — цепочки эффектов для голоса: фильтры, перегруз, биткраш,
// компрессор, гейт, «рация» (щелчки/шипение), выпадения сигнала, робот.
package dsp

import (
	"fmt"
	"math"
	"math/rand"
)

// Effect обрабатывает моно-буфер на месте.
type Effect interface {
	Process(buf []float32)
}

// Spec — описание эффекта в JSON-конфиге.
type Spec struct {
	Type string `json:"type"`
	// общие параметры (используются в зависимости от Type)
	Freq      float64 `json:"freq,omitempty"`
	Q         float64 `json:"q,omitempty"`
	DB        float64 `json:"db,omitempty"`
	Amount    float64 `json:"amount,omitempty"`
	Mode      string  `json:"mode,omitempty"`
	Bits      int     `json:"bits,omitempty"`
	Down      int     `json:"downsample,omitempty"`
	Threshold float64 `json:"threshold_db,omitempty"`
	Ratio     float64 `json:"ratio,omitempty"`
	AttackMS  float64 `json:"attack_ms,omitempty"`
	ReleaseMS float64 `json:"release_ms,omitempty"`
	MakeupDB  float64 `json:"makeup_db,omitempty"`
	HangMS    float64 `json:"hang_ms,omitempty"`
	ClickDB   float64 `json:"click_db,omitempty"`
	TailMS    float64 `json:"tail_ms,omitempty"`
	TailDB    float64 `json:"tail_db,omitempty"`
	StaticDB  float64 `json:"static_db,omitempty"`
	Chance    float64 `json:"chance,omitempty"`
	LenMS     float64 `json:"len_ms,omitempty"`
	Mix       float64 `json:"mix,omitempty"`
	Kind      string  `json:"kind,omitempty"`
}

func dbToLin(db float64) float64 { return math.Pow(10, db/20) }

// Build создаёт эффект по описанию.
func Build(s Spec, sr float64) (Effect, error) {
	q := s.Q
	if q == 0 {
		q = 0.707
	}
	switch s.Type {
	case "gain":
		return &Gain{G: float32(dbToLin(s.DB))}, nil
	case "lowpass", "highpass", "bandpass", "peaking", "lowshelf", "highshelf":
		if s.Freq <= 0 {
			return nil, fmt.Errorf("%s: нужен freq", s.Type)
		}
		return NewBiquad(s.Type, s.Freq, q, s.DB, sr), nil
	case "drive":
		a := s.Amount
		if a == 0 {
			a = 0.7
		}
		return &Drive{Amount: a, Mode: s.Mode}, nil
	case "bitcrush":
		b := s.Bits
		if b == 0 {
			b = 8
		}
		d := s.Down
		if d < 1 {
			d = 1
		}
		return &Bitcrush{Bits: b, Down: d}, nil
	case "compressor":
		r := s.Ratio
		if r == 0 {
			r = 4
		}
		return NewCompressor(s.Threshold, r, or(s.AttackMS, 5), or(s.ReleaseMS, 80), s.MakeupDB, sr), nil
	case "gate":
		return NewGate(s.Threshold, or(s.HangMS, 120), sr), nil
	case "squelch":
		return NewSquelch(s, sr), nil
	case "noise":
		return &Noise{Level: float32(dbToLin(or(s.DB, -45))), Crackle: s.Kind == "crackle"}, nil
	case "dropout":
		return &Dropout{Chance: or(s.Chance, 0.01), Len: int(or(s.LenMS, 50) * sr / 1000)}, nil
	case "ringmod":
		return &RingMod{Freq: or(s.Freq, 50), Mix: or(s.Mix, 0.7), sr: sr}, nil
	case "limiter":
		return NewCompressor(or(s.Threshold, -1), 50, 0.5, 60, 0, sr), nil
	}
	return nil, fmt.Errorf("неизвестный эффект %q", s.Type)
}

func or(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

// ---------- простые эффекты ----------

type Gain struct{ G float32 }

func (g *Gain) Process(b []float32) {
	for i := range b {
		b[i] *= g.G
	}
}

// Drive — перегруз. mode: soft (tanh), hard (жёсткий клиппинг), fold (заворот волны).
type Drive struct {
	Amount float64
	Mode   string
}

func (d *Drive) Process(b []float32) {
	k := 1 + d.Amount*20
	norm := math.Tanh(k)
	for i, x := range b {
		v := float64(x)
		switch d.Mode {
		case "hard":
			v *= k
			if v > 1 {
				v = 1
			} else if v < -1 {
				v = -1
			}
		case "fold":
			v *= k
			for v > 1 || v < -1 {
				if v > 1 {
					v = 2 - v
				} else {
					v = -2 - v
				}
			}
		default:
			v = math.Tanh(v*k) / norm
		}
		b[i] = float32(v)
	}
}

// Bitcrush — понижение разрядности и частоты дискретизации.
type Bitcrush struct {
	Bits, Down int
	hold       float32
	cnt        int
}

func (c *Bitcrush) Process(b []float32) {
	steps := float32(math.Pow(2, float64(c.Bits-1)))
	for i, x := range b {
		if c.cnt%c.Down == 0 {
			c.hold = float32(math.Round(float64(x*steps))) / steps
		}
		c.cnt++
		b[i] = c.hold
	}
}

type Noise struct {
	Level   float32
	Crackle bool
}

func (n *Noise) Process(b []float32) {
	for i := range b {
		v := rand.Float32()*2 - 1
		if n.Crackle {
			if rand.Float32() < 0.002 {
				v *= 8
			} else {
				v *= 0.3
			}
		}
		b[i] += v * n.Level
	}
}

// Dropout — случайные выпадения сигнала, как у плохой связи.
type Dropout struct {
	Chance float64
	Len    int
	left   int
}

func (d *Dropout) Process(b []float32) {
	for i := range b {
		if d.left > 0 {
			d.left--
			b[i] *= 0.05
			continue
		}
		if rand.Float64() < d.Chance/float64(d.Len+1) {
			d.left = d.Len/2 + rand.Intn(d.Len+1)
		}
	}
}

// RingMod — «робот».
type RingMod struct {
	Freq, Mix float64
	sr, ph    float64
}

func (r *RingMod) Process(b []float32) {
	inc := 2 * math.Pi * r.Freq / r.sr
	for i, x := range b {
		m := math.Sin(r.ph)
		r.ph += inc
		if r.ph > 2*math.Pi {
			r.ph -= 2 * math.Pi
		}
		b[i] = float32(float64(x)*(1-r.Mix) + float64(x)*m*r.Mix)
	}
}

// ---------- биквадратный фильтр (RBJ cookbook) ----------

type Biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func NewBiquad(kind string, f, q, gainDB, sr float64) *Biquad {
	w := 2 * math.Pi * f / sr
	cw, sw := math.Cos(w), math.Sin(w)
	alpha := sw / (2 * q)
	A := math.Pow(10, gainDB/40)
	var b0, b1, b2, a0, a1, a2 float64
	switch kind {
	case "lowpass":
		b0, b1, b2 = (1-cw)/2, 1-cw, (1-cw)/2
		a0, a1, a2 = 1+alpha, -2*cw, 1-alpha
	case "highpass":
		b0, b1, b2 = (1+cw)/2, -(1 + cw), (1+cw)/2
		a0, a1, a2 = 1+alpha, -2*cw, 1-alpha
	case "bandpass":
		b0, b1, b2 = alpha, 0, -alpha
		a0, a1, a2 = 1+alpha, -2*cw, 1-alpha
	case "peaking":
		b0, b1, b2 = 1+alpha*A, -2*cw, 1-alpha*A
		a0, a1, a2 = 1+alpha/A, -2*cw, 1-alpha/A
	case "lowshelf":
		sq := 2 * math.Sqrt(A) * alpha
		b0 = A * ((A + 1) - (A-1)*cw + sq)
		b1 = 2 * A * ((A - 1) - (A+1)*cw)
		b2 = A * ((A + 1) - (A-1)*cw - sq)
		a0 = (A + 1) + (A-1)*cw + sq
		a1 = -2 * ((A - 1) + (A+1)*cw)
		a2 = (A + 1) + (A-1)*cw - sq
	case "highshelf":
		sq := 2 * math.Sqrt(A) * alpha
		b0 = A * ((A + 1) + (A-1)*cw + sq)
		b1 = -2 * A * ((A - 1) + (A+1)*cw)
		b2 = A * ((A + 1) + (A-1)*cw - sq)
		a0 = (A + 1) - (A-1)*cw + sq
		a1 = 2 * ((A - 1) - (A+1)*cw)
		a2 = (A + 1) - (A-1)*cw - sq
	}
	return &Biquad{b0: b0 / a0, b1: b1 / a0, b2: b2 / a0, a1: a1 / a0, a2: a2 / a0}
}

func (f *Biquad) Process(b []float32) {
	for i, xf := range b {
		x := float64(xf)
		y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
		f.x2, f.x1 = f.x1, x
		f.y2, f.y1 = f.y1, y
		b[i] = float32(y)
	}
}

// ---------- динамика ----------

type Compressor struct {
	thr, ratio, att, rel, makeup float64
	env                          float64
}

func NewCompressor(thrDB, ratio, attMS, relMS, makeupDB, sr float64) *Compressor {
	return &Compressor{
		thr: thrDB, ratio: ratio,
		att:    math.Exp(-1 / (attMS * sr / 1000)),
		rel:    math.Exp(-1 / (relMS * sr / 1000)),
		makeup: dbToLin(makeupDB),
	}
}

func (c *Compressor) Process(b []float32) {
	for i, xf := range b {
		x := math.Abs(float64(xf))
		if x > c.env {
			c.env = c.att*c.env + (1-c.att)*x
		} else {
			c.env = c.rel*c.env + (1-c.rel)*x
		}
		g := 1.0
		if lv := 20 * math.Log10(c.env+1e-9); lv > c.thr {
			g = dbToLin((c.thr + (lv-c.thr)/c.ratio) - lv)
		}
		b[i] = float32(float64(xf) * g * c.makeup)
	}
}

// Gate — шумоподавитель: глушит всё тише порога (важно перед сильным перегрузом).
type Gate struct {
	thr         float64
	env, g      float64
	hang, hangN int
}

func NewGate(thrDB, hangMS, sr float64) *Gate {
	if thrDB == 0 {
		thrDB = -50
	}
	return &Gate{thr: dbToLin(thrDB), hangN: int(hangMS * sr / 1000)}
}

// Open возвращает true, если гейт открыт (есть голос).
func (g *Gate) step(x float64) bool {
	g.env = math.Max(math.Abs(x), g.env*0.9995)
	if g.env > g.thr {
		g.hang = g.hangN
	} else if g.hang > 0 {
		g.hang--
	}
	return g.hang > 0
}

func (g *Gate) Process(b []float32) {
	for i, x := range b {
		target := 0.0
		if g.step(float64(x)) {
			target = 1
		}
		g.g += (target - g.g) * 0.005
		b[i] = float32(float64(x) * g.g)
	}
}

// Squelch — «рация»: гейт + щелчок при начале фразы, шипение во время речи
// и хвост шума (roger) после окончания фразы.
type Squelch struct {
	gate                       *Gate
	open                       bool
	click, tail                []float32
	burst                      []float32
	bpos                       int
	staticLvl, clickLvl, tailL float32
	g                          float64
}

func NewSquelch(s Spec, sr float64) *Squelch {
	q := &Squelch{
		gate:      NewGate(or(s.Threshold, -45), or(s.HangMS, 220), sr),
		staticLvl: float32(dbToLin(or(s.StaticDB, -38))),
		clickLvl:  float32(dbToLin(or(s.ClickDB, -10))),
		tailL:     float32(dbToLin(or(s.TailDB, -16))),
	}
	// щелчок: короткий импульс + затухающий шум 12 мс
	n := int(0.012 * sr)
	q.click = make([]float32, n)
	for i := range q.click {
		e := float32(math.Exp(-float64(i) / (0.002 * sr)))
		q.click[i] = (rand.Float32()*2 - 1) * e
	}
	q.click[0], q.click[1] = 1, -0.8
	// хвост: полоса шума
	tn := int(or(s.TailMS, 140) / 1000 * sr)
	q.tail = make([]float32, tn)
	for i := range q.tail {
		e := float32(1 - float64(i)/float64(tn))
		q.tail[i] = (rand.Float32()*2 - 1) * e
	}
	return q
}

func (q *Squelch) Process(b []float32) {
	for i, x := range b {
		open := q.gate.step(float64(x))
		if open && !q.open {
			q.burst, q.bpos = q.click, 0
		} else if !open && q.open {
			q.burst, q.bpos = q.tail, 0
		}
		q.open = open
		target := 0.0
		if open {
			target = 1
		}
		q.g += (target - q.g) * 0.01
		v := x * float32(q.g)
		if open {
			v += (rand.Float32()*2 - 1) * q.staticLvl
		}
		if q.burst != nil {
			lvl := q.clickLvl
			if len(q.burst) == len(q.tail) {
				lvl = q.tailL
			}
			v += q.burst[q.bpos] * lvl
			q.bpos++
			if q.bpos >= len(q.burst) {
				q.burst = nil
			}
		}
		b[i] = v
	}
}

// ---------- цепочка ----------

type Chain struct {
	Name    string
	Effects []Effect
}

func (c *Chain) Process(b []float32) {
	if c == nil {
		return
	}
	for _, e := range c.Effects {
		e.Process(b)
	}
	for i, x := range b { // защита от NaN/взрыва фильтров
		if x != x || x > 4 || x < -4 {
			b[i] = 0
		}
	}
}

// BuildChain собирает цепочку из описаний.
func BuildChain(name string, specs []Spec, sr float64) (*Chain, error) {
	c := &Chain{Name: name}
	for _, s := range specs {
		e, err := Build(s, sr)
		if err != nil {
			return nil, fmt.Errorf("пресет %s: %w", name, err)
		}
		c.Effects = append(c.Effects, e)
	}
	return c, nil
}
