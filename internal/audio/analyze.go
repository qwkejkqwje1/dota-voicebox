package audio

import (
	"math"
	"sort"
)

// Stats — анализ записи (уровни в dBFS: 0 = максимум, -60 = почти тишина).
type Stats struct {
	Seconds     float64 `json:"seconds"`
	PeakDB      float64 `json:"peak_db"`
	RMSDB       float64 `json:"rms_db"`
	NoiseDB     float64 `json:"noise_db"`  // фон (тихие участки)
	SpeechDB    float64 `json:"speech_db"` // громкие участки (речь)
	ClipPct     float64 `json:"clip_pct"`  // доля перегруженных сэмплов, %
	Silent      bool    `json:"silent"`
	SuggestGain float64 `json:"suggest_gain,omitempty"`
}

func DB(x float64) float64 {
	if x < 1e-6 {
		return -120
	}
	return math.Round(20*math.Log10(x)*10) / 10
}

// Analyze считает уровни записи с частотой sr.
func Analyze(b []float32, sr int) Stats {
	st := Stats{Seconds: float64(len(b)) / float64(sr)}
	if len(b) == 0 {
		st.Silent, st.PeakDB, st.RMSDB, st.NoiseDB, st.SpeechDB = true, -120, -120, -120, -120
		return st
	}
	var peak, sum float64
	clip := 0
	for _, x := range b {
		a := math.Abs(float64(x))
		if a > peak {
			peak = a
		}
		if a >= 0.99 {
			clip++
		}
		sum += a * a
	}
	st.PeakDB = DB(peak)
	st.RMSDB = DB(math.Sqrt(sum / float64(len(b))))
	st.ClipPct = math.Round(float64(clip)/float64(len(b))*100*1000) / 1000
	// окна по 50 мс
	w := sr / 20
	var win []float64
	for i := 0; i+w <= len(b); i += w {
		var s float64
		for _, x := range b[i : i+w] {
			s += float64(x) * float64(x)
		}
		win = append(win, math.Sqrt(s/float64(w)))
	}
	if len(win) > 0 {
		sort.Float64s(win)
		st.NoiseDB = DB(win[len(win)/10])
		st.SpeechDB = DB(win[len(win)*9/10])
	}
	st.Silent = st.PeakDB < -55
	// цель — речь около -18 dBFS
	if !st.Silent && st.SpeechDB > -70 {
		g := math.Pow(10, (-18-st.SpeechDB)/20)
		if st.ClipPct > 0.05 {
			g = math.Min(g, 0.7)
		}
		st.SuggestGain = math.Round(math.Max(0.3, math.Min(4, g))*10) / 10
	}
	return st
}

// ToneOnset ищет начало тона частоты freq (алгоритм Гёрцеля, окна 5 мс).
// Возвращает время начала в секундах и уровень тона (dBFS), или -1, если тона нет.
func ToneOnset(b []float32, sr int, freq float64) (onset float64, levelDB float64) {
	w := sr / 200
	if w < 8 {
		w = 8
	}
	coeff := 2 * math.Cos(2*math.Pi*freq/float64(sr))
	var powers []float64
	for i := 0; i+w <= len(b); i += w {
		var s1, s2 float64
		var energy float64
		for _, x := range b[i : i+w] {
			s := float64(x) + coeff*s1 - s2
			s2, s1 = s1, s
			energy += float64(x) * float64(x)
		}
		p := s1*s1 + s2*s2 - coeff*s1*s2 // ≈ (A·w/2)²
		amp := 2 * math.Sqrt(math.Max(p, 0)) / float64(w)
		// тон должен быть основной частью энергии окна (а не речь/шум)
		rms := math.Sqrt(energy / float64(w))
		if rms > 0 && amp/math.Sqrt2 < rms*0.6 {
			amp = 0
		}
		powers = append(powers, amp)
	}
	const minAmp = 0.003 // ≈ -50 dBFS
	for i := 0; i+2 < len(powers); i++ {
		if powers[i] > minAmp && powers[i+1] > minAmp && powers[i+2] > minAmp {
			// уровень — максимум амплитуды на следующих 100 мс
			var m float64
			for j := i; j < len(powers) && j < i+20; j++ {
				m = math.Max(m, powers[j])
			}
			return float64(i*w) / float64(sr), DB(m)
		}
	}
	return -1, -120
}

// Tone — синус частоты freq длительностью sec с плавными краями.
func Tone(freq, sec, amp float64, sr int) []float32 {
	n := int(sec * float64(sr))
	out := make([]float32, n)
	fade := sr / 100
	for i := range out {
		g := amp
		if i < fade {
			g *= float64(i) / float64(fade)
		}
		if n-i < fade {
			g *= float64(n-i) / float64(fade)
		}
		out[i] = float32(g * math.Sin(2*math.Pi*freq*float64(i)/float64(sr)))
	}
	return out
}
