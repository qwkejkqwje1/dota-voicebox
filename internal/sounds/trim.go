package sounds

import (
	"math"
	"sort"
	"strings"
)

// Обрезка звуков и данные для редактора: волна и поиск тишины по краям.

// Trimmable — можно ли обрезать звук: ровно один аудиофайл (не встроенный звук и не фраза).
func Trimmable(files []string) bool {
	return len(files) == 1 && files[0] != "" && !strings.HasPrefix(files[0], "builtin:") && !strings.HasPrefix(files[0], "tts:")
}

// MinTrim — самый короткий кусок, который можно оставить при обрезке, сек.
const MinTrim = 0.05

// TrimSpan переводит [start, end) секунд (end <= 0 — до конца) в номера сэмплов.
// Неправильный или слишком короткий кусок — весь клип.
func TrimSpan(n int, start, end float64) (int, int) {
	a, b := 0, n
	if start > 0 {
		a = int(math.Round(start * SampleRate))
	}
	if end > 0 {
		b = int(math.Round(end * SampleRate))
	}
	a, b = max(0, min(a, n)), max(0, min(b, n))
	if b-a < int(MinTrim*SampleRate) {
		return 0, n
	}
	return a, b
}

// TrimClip вырезает кусок [start, end) секунд (end <= 0 — до конца) — новой копией.
// На краях среза — короткое затухание (4 мс), чтобы не было щелчков.
func TrimClip(c Clip, start, end float64) Clip {
	a, b := TrimSpan(len(c), start, end)
	out := append(Clip(nil), c[a:b]...)
	const fade = SampleRate * 4 / 1000
	if a > 0 {
		for i := 0; i < fade && i < len(out); i++ {
			out[i] *= float32(i) / fade
		}
	}
	if b < len(c) {
		for i := 0; i < fade && i < len(out); i++ {
			out[len(out)-1-i] *= float32(i) / fade
		}
	}
	return out
}

// Peaks — огибающая для рисования волны: n значений 0..1 (максимум модуля на каждом отрезке).
func Peaks(c Clip, n int) []float32 {
	if n <= 0 || len(c) == 0 {
		return []float32{}
	}
	out := make([]float32, n)
	for i := range out {
		a, b := i*len(c)/n, (i+1)*len(c)/n
		if b <= a {
			b = min(a+1, len(c))
		}
		var m float32
		for _, x := range c[a:b] {
			if x < 0 {
				x = -x
			}
			if x > m {
				m = x
			}
		}
		out[i] = min(m, 1)
	}
	return out
}

// FindSilence ищет, где в клипе начинается и кончается сам звук (сек).
// Тихо — заметно ниже самого громкого места и не громче фонового шума записи.
// Короткие щелчки по краям (мышь, кнопка записи — короче 40 мс и отдельно от звука)
// звуком не считаются, если в записи есть что-то длиннее. По краям — небольшой запас.
// Если звука нет совсем, возвращает весь клип.
func FindSilence(c Clip) (start, end float64) {
	const (
		win    = SampleRate / 100 // окно 10 мс
		gap    = 10               // паузы короче 100 мс (между слогами) не разрывают звук
		minLen = 4                // меньше 40 мс громкого — щелчок
	)
	dur := float64(len(c)) / SampleRate
	frames := (len(c) + win - 1) / win
	if frames == 0 {
		return 0, 0
	}
	rms := make([]float64, frames)
	var loud float64
	for f := range rms {
		a, b := f*win, min((f+1)*win, len(c))
		var s float64
		for _, x := range c[a:b] {
			s += float64(x) * float64(x)
		}
		rms[f] = math.Sqrt(s / float64(b-a))
		loud = math.Max(loud, rms[f])
	}
	if loud < 1e-4 {
		return 0, dur
	}
	sorted := append([]float64(nil), rms...)
	sort.Float64s(sorted)
	floor := sorted[len(sorted)/10] // фон: самые тихие 10 % записи
	thr := math.Max(math.Max(loud*0.02, floor*3), 3e-4)
	type span struct{ a, b, on int } // окна [a, b), из них громких — on
	var parts []span
	for f, v := range rms {
		if v < thr {
			continue
		}
		if n := len(parts); n > 0 && f-parts[n-1].b < gap {
			parts[n-1].b = f + 1
			parts[n-1].on++
		} else {
			parts = append(parts, span{f, f + 1, 1})
		}
	}
	var long []span
	for _, p := range parts {
		if p.on >= minLen {
			long = append(long, p)
		}
	}
	if len(long) == 0 { // только короткие звуки (хлопок) — их и оставляем
		long = parts
	}
	if len(long) == 0 {
		return 0, dur
	}
	start = math.Max(0, float64(long[0].a*win)/SampleRate-0.08)
	end = math.Min(dur, float64(long[len(long)-1].b*win)/SampleRate+0.15)
	return math.Round(start*1000) / 1000, math.Round(end*1000) / 1000
}

// PeakNormalize поднимает громкость так, чтобы пик был target (не больше чем в maxGain раз).
// Почти тишину не трогает — иначе вылезет только шум.
func PeakNormalize(c Clip, target, maxGain float32) {
	var pk float32
	for _, x := range c {
		if x < 0 {
			x = -x
		}
		pk = max(pk, x)
	}
	if pk < 0.003 {
		return
	}
	g := min(target/pk, maxGain)
	for i := range c {
		c[i] *= g
	}
}
