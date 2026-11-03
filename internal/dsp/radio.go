package dsp

import "math"

// Radio — имитация портативной рации («уоки-токи»), собранная как настоящий тракт связи.
//
//	Передатчик: срез гула, АРУ (компрессия), перегруз с асимметрией, полоса 4-го порядка,
//	            резонанс маленького динамика, по желанию — цифровая грязь (частота и разрядность).
//	Эфир:       шипение несущей (тише под голосом — «захват» FM), помехи: треск, выпадения, замирания.
//	Приёмник:   шумоподавитель (squelch) — щелчок и «кшш» в начале передачи, хвост «пшш» после неё.
//	Сигналы:    тон разрешения передачи (TPT), «роджер-бип» в конце, тоны Квиндара (ЦУП NASA).
//
// Передача начинается по тангенте (SetKey, режим «говорю по кнопке») или по голосу:
// детектор речи по RMS окнами по 6 мс с гистерезисом; щелчки клавиатуры его не открывают,
// а упреждение 18 мс не даёт съесть начало первого слова.
type Radio struct {
	sr float64

	k, bias, tb, norm float64 // перегруз
	drive             bool
	noise             float32 // множитель шипения
	interf            float64
	holdN             int
	steps             float64
	squelch, tpt      bool
	beep              string
	openThr, closeThr float64
	hangW             int

	extKey, extKnown bool
	tx               bool
	acc              float64
	cnt, winN        int
	aboveW, hang     int
	gate, gateStep   float32

	pre, vadHP *Biquad
	comp       *Compressor
	band       [4]*Biquad
	presence   *Biquad
	digLP      *Biquad
	nzHP, nzLP *Biquad
	bzHP, bzLP *Biquad
	look       *dline
	lookN      float64

	tones       []toneSeg
	tIdx, tPos  int
	tPh         float64
	pendingTail bool

	burstN, burstPos  int
	burstLvl          float32
	burstAtk, burstPl float64
	burstDec          float64
	bzGain            float32
	click             int

	r               rng
	crack           float32
	dropLeft        int
	dropG           float32
	fadePh, fadeInc float64
	voiceEnv        float32
	holdCnt         int
	held            float64
}

type toneSeg struct {
	f0, f1 float64 // Гц (свип от f0 к f1); 0 — пауза
	n      int
	amp    float64
}

// NewRadio собирает рацию по описанию (type "radio").
func NewRadio(s Spec, sr float64) *Radio {
	lo, hi := or(s.LowHz, 300), or(s.HighHz, 3000)
	r := &Radio{
		sr: sr, interf: clamp01(s.Interference), squelch: !s.NoSquelch, tpt: s.TPT, beep: s.Beep,
		openThr: dbToLin(or(s.Threshold, -40)), hangW: int(or(s.HangMS, 380) / 6),
		winN: int(0.006 * sr), gateStep: float32(1 / (0.003 * sr)),
		pre: NewBiquad("highpass", 120, 0.707, 0, sr), vadHP: NewBiquad("highpass", 200, 0.707, 0, sr),
		comp:     NewCompressor(-30, 5, 3, 150, 16, sr),
		presence: NewBiquad("peaking", 1800, 1.1, 4, sr),
		nzHP:     NewBiquad("highpass", lo, 0.707, 0, sr), nzLP: NewBiquad("lowpass", math.Min(hi*1.15, sr*0.45), 0.707, 0, sr),
		bzHP: NewBiquad("highpass", 400, 0.707, 0, sr), bzLP: NewBiquad("lowpass", 4500, 0.707, 0, sr),
		lookN: 0.018 * sr, dropG: 1, fadeInc: 2 * math.Pi * 0.6 / sr, r: rng{s: 0x1234567},
	}
	r.closeThr = r.openThr * 0.5
	r.look = newDline(int(r.lookN) + 4)
	// Баттерворт 4-го порядка = два звена с Q 0.541 и 1.307
	r.band = [4]*Biquad{
		NewBiquad("highpass", lo, 0.5412, 0, sr), NewBiquad("highpass", lo, 1.3066, 0, sr),
		NewBiquad("lowpass", hi, 0.5412, 0, sr), NewBiquad("lowpass", hi, 1.3066, 0, sr),
	}
	if d := clamp01(s.Drive); d > 0 {
		r.drive = true
		r.k = 1 + d*8
		r.bias = 0.12 * d
		r.tb = math.Tanh(r.k * r.bias)
		r.norm = 0.5 / math.Tanh(r.k*0.3)
	}
	if s.NoiseDB < 0 {
		bw := math.Max(200, math.Min(hi*1.15, sr*0.45)-lo)
		r.noise = float32(dbToLin(s.NoiseDB) / (0.577 * math.Sqrt(bw/(sr/2))))
	}
	r.bzGain = float32(1 / (0.577 * math.Sqrt(4100/(sr/2))))
	if dg := clamp01(s.Digital); dg > 0 {
		r.holdN = 1 + int(dg*4.5)
		r.steps = math.Pow(2, 13-dg*7)
		r.digLP = NewBiquad("lowpass", hi, 0.707, 0, sr)
	}
	return r
}

func (r *Radio) SetKey(open, known bool) { r.extKey, r.extKnown = open, known }

// Transmitting — идёт ли сейчас передача (для индикатора).
func (r *Radio) Transmitting() bool { return r.tx }

func (r *Radio) ms(v float64) int { return int(v * r.sr / 1000) }

func (r *Radio) startTX() {
	r.tx = true
	r.tones, r.tIdx, r.tPos = nil, 0, 0
	r.pendingTail = false
	if r.squelch {
		r.click = 3
		r.burst(32, -15, 0.4, 0, 9)
	}
	switch {
	case r.beep == "quindar":
		r.tones = []toneSeg{{2525, 2525, r.ms(250), 0.28}}
	case r.tpt:
		r.tones = []toneSeg{{1000, 1000, r.ms(45), 0.28}, {0, 0, r.ms(20), 0}, {1500, 1500, r.ms(45), 0.28}}
	}
}

func (r *Radio) stopTX() {
	r.tx = false
	gap := toneSeg{0, 0, r.ms(30), 0}
	switch r.beep {
	case "beep":
		r.tones = []toneSeg{gap, {1250, 1250, r.ms(110), 0.3}}
	case "double":
		r.tones = []toneSeg{gap, {1000, 1000, r.ms(65), 0.3}, {0, 0, r.ms(35), 0}, {1450, 1450, r.ms(75), 0.3}}
	case "chirp":
		r.tones = []toneSeg{gap, {800, 2000, r.ms(130), 0.3}}
	case "quindar":
		r.tones = []toneSeg{gap, {2475, 2475, r.ms(250), 0.28}}
	default:
		r.tones = nil
	}
	r.tIdx, r.tPos = 0, 0
	r.pendingTail = r.squelch
}

// burst — шумовой всплеск шумоподавителя: длина, уровень (дБ), атака, «полка» и спад в мс.
func (r *Radio) burst(lenMS, db, atkMS, plateauMS, decMS float64) {
	r.burstN, r.burstPos = r.ms(lenMS), 0
	r.burstLvl = float32(dbToLin(db)) * r.bzGain
	r.burstAtk, r.burstPl, r.burstDec = atkMS*r.sr/1000, plateauMS*r.sr/1000, decMS*r.sr/1000
}

func (r *Radio) toneTick() float64 {
	if r.tIdx >= len(r.tones) {
		return 0
	}
	s := &r.tones[r.tIdx]
	var y float64
	if s.f0 > 0 {
		f := s.f0 + (s.f1-s.f0)*float64(r.tPos)/float64(s.n)
		r.tPh += 2 * math.Pi * f / r.sr
		if r.tPh > 2*math.Pi {
			r.tPh -= 2 * math.Pi
		}
		ramp := math.Min(1, float64(min(r.tPos, s.n-r.tPos))/(0.003*r.sr))
		y = math.Sin(r.tPh) * s.amp * ramp
	}
	if r.tPos++; r.tPos >= s.n {
		r.tIdx++
		r.tPos = 0
	}
	return y
}

func (r *Radio) Process(b []float32) {
	for i, x := range b {
		// --- тангента / детектор речи ---
		h := r.vadHP.tick(float64(x))
		r.acc += h * h
		if r.cnt++; r.cnt >= r.winN {
			rms := math.Sqrt(r.acc / float64(r.winN))
			r.acc, r.cnt = 0, 0
			if rms > r.openThr {
				r.aboveW++
			} else {
				r.aboveW = 0
			}
			switch {
			case r.aboveW >= 3 || (r.tx && rms > r.closeThr):
				r.hang = r.hangW
			case r.hang > 0:
				r.hang--
			}
		}
		want := r.hang > 0
		if r.extKnown {
			want = r.extKey
		}
		if want != r.tx {
			if want {
				r.startTX()
			} else {
				r.stopTX()
			}
		}
		if r.tx {
			r.gate = min(1, r.gate+r.gateStep)
		} else {
			r.gate = max(0, r.gate-r.gateStep)
		}

		// --- передатчик ---
		r.look.push(x)
		v := float64(r.look.tap(r.lookN) * r.gate)
		v = r.pre.tick(v)
		v = r.comp.tick(v)
		v += r.toneTick()
		if r.drive {
			v = (math.Tanh(r.k*(v+r.bias)) - r.tb) * r.norm
		}
		for _, f := range r.band {
			v = f.tick(v)
		}
		v = r.presence.tick(v)
		if r.holdN > 0 {
			if r.holdCnt%r.holdN == 0 {
				r.held = math.Round(v*r.steps) / r.steps
			}
			r.holdCnt++
			v = r.digLP.tick(r.held)
		}

		// --- эфир ---
		carrier := r.tx || r.tIdx < len(r.tones)
		av := float32(math.Abs(v))
		r.voiceEnv = max(av, r.voiceEnv*0.9995)
		var out, hiss float64
		if carrier {
			if r.noise > 0 {
				n := r.nzLP.tick(r.nzHP.tick(float64(r.r.bi())))
				duck := math.Min(1, float64(r.voiceEnv)/0.25)
				hiss = n * float64(r.noise) * (1 - 0.6*duck)
			}
			if r.interf > 0 {
				r.fadePh += r.fadeInc * (0.7 + 0.6*r.r.unit())
				if r.fadePh > 2*math.Pi {
					r.fadePh -= 2 * math.Pi
				}
				v *= 1 - r.interf*0.45*(0.5+0.5*math.Sin(r.fadePh))
				target := float32(1)
				if r.dropLeft > 0 {
					r.dropLeft--
					target = 0.06
				} else if r.r.unit() < r.interf*0.9/r.sr {
					r.dropLeft = r.ms(40 + 120*r.r.unit())
				}
				r.dropG += (target - r.dropG) * 0.004
				v *= float64(r.dropG)
				hiss *= 1 + 2.5*float64(1-r.dropG)
				if r.r.unit() < r.interf*40/r.sr {
					r.crack = (0.25 + 0.45*float32(r.r.unit())) * float32(1-2*float64(r.r.next()&1))
				}
			}
		}
		out = v + hiss + float64(r.crack)
		r.crack *= 0.7

		// --- приёмник: шумоподавитель ---
		if r.pendingTail && !r.tx && r.tIdx >= len(r.tones) {
			r.pendingTail = false
			r.burst(200, -11, 1, 95, 35)
		}
		if r.burstPos < r.burstN {
			t := float64(r.burstPos)
			env := 1 - math.Exp(-t/r.burstAtk)
			if t > r.burstPl {
				env *= math.Exp(-(t - r.burstPl) / r.burstDec)
			}
			n := r.bzLP.tick(r.bzHP.tick(float64(r.r.bi())))
			out += n * float64(r.burstLvl) * env
			r.burstPos++
		}
		if r.click > 0 {
			out += [...]float64{0.25, -0.6, 0.9}[r.click-1] * 0.5
			r.click--
		}
		b[i] = float32(math.Tanh(out))
	}
}
