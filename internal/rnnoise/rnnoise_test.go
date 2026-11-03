package rnnoise

import (
	"math"
	"testing"
)

// voice — «голос»: гармоники с формантами и слогами; noise — белый шум.
func voice(n int) []float32 {
	out := make([]float32, n)
	ph := 0.0
	for i := range out {
		t := float64(i) / 48000
		f0 := 140 + 30*math.Sin(2*math.Pi*0.7*t)
		ph += 2 * math.Pi * f0 / 48000
		v := 0.0
		for k := 1; k < 30; k++ {
			fk := float64(k) * f0
			form := math.Exp(-math.Pow((fk-700)/300, 2)) + 0.6*math.Exp(-math.Pow((fk-1200)/400, 2)) + 0.3*math.Exp(-math.Pow((fk-2600)/500, 2))
			v += form * math.Sin(float64(k)*ph) / float64(k)
		}
		env := 0.5 + 0.5*math.Sin(2*math.Pi*3*t)
		if env < 0.2 {
			env = 0
		}
		out[i] = float32(v * env * 0.3)
	}
	return out
}

func noise(n int, amp float32) []float32 {
	out := make([]float32, n)
	s := uint32(7)
	for i := range out {
		s = s*1664525 + 1013904223
		out[i] = (float32(s>>9)/8388608 - 1) * amp
	}
	return out
}

func energy(b []float32) float64 {
	e := 0.0
	for _, x := range b {
		e += float64(x) * float64(x)
	}
	return e
}

// run прогоняет сигнал кусками разной длины.
func run(s *Stream, in []float32, on bool, strength float32) []float32 {
	out := append([]float32(nil), in...)
	sizes := []int{441, 480, 512, 300, 960}
	for i, k := 0, 0; i < len(out); k++ {
		n := min(sizes[k%len(sizes)], len(out)-i)
		s.Process(out[i:i+n], on, strength)
		i += n
	}
	return out
}

func TestNoiseRemovedVoiceKept(t *testing.T) {
	s, err := NewStream()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const n = 48000 * 3
	nz := run(s, noise(n, 0.05), true, 1)
	if db := 10 * math.Log10(energy(nz[48000:])/energy(noise(n, 0.05)[48000:])); db > -20 {
		t.Fatalf("шум ослаблен только на %.1f дБ", db)
	}
	s2, _ := NewStream()
	defer s2.Close()
	v := voice(n)
	vo := run(s2, v, true, 1)
	if db := 10 * math.Log10(energy(vo[48000:])/energy(v[48000:])); db < -3 {
		t.Fatalf("голос стал тише на %.1f дБ", -db)
	}
}

func TestBypassAndDryAlignment(t *testing.T) {
	s, _ := NewStream()
	defer s.Close()
	in := voice(48000)
	if out := run(s, in, false, 1); energy(diff(out, in)) != 0 {
		t.Fatal("выключенное шумоподавление должно пропускать сигнал без изменений")
	}
	// сила 0 — на выходе исходный сигнал с задержкой сети и очереди (куски не по 10 мс → +1 кадр)
	s2, _ := NewStream()
	defer s2.Close()
	out := run(s2, in, true, 0)
	lag := Delay + FrameSize
	tail := out[lag+fadeLen:]
	ref := in[fadeLen : fadeLen+len(tail)]
	if e := energy(diff(tail, ref)); e > 1e-9 {
		t.Fatalf("сухой сигнал не выровнен: ошибка %g", e)
	}
}

func TestAlignedChunksNoExtraDelay(t *testing.T) {
	s, _ := NewStream()
	defer s.Close()
	in := voice(48000)
	out := append([]float32(nil), in...)
	for i := 0; i < len(out); i += FrameSize {
		s.Process(out[i:i+FrameSize], true, 0)
	}
	tail := out[Delay+fadeLen:]
	if e := energy(diff(tail, in[fadeLen:fadeLen+len(tail)])); e > 1e-9 {
		t.Fatalf("куски по 10 мс не должны добавлять задержку: ошибка %g", e)
	}
}

func TestNoAllocs(t *testing.T) {
	s, _ := NewStream()
	defer s.Close()
	buf := voice(441)
	s.Process(buf, true, 1)
	if a := testing.AllocsPerRun(50, func() { s.Process(buf, true, 0.8) }); a != 0 {
		t.Fatalf("выделения памяти в аудиопотоке: %v", a)
	}
}

func diff(a, b []float32) []float32 {
	out := make([]float32, min(len(a), len(b)))
	for i := range out {
		out[i] = a[i] - b[i]
	}
	return out
}

func BenchmarkStream(b *testing.B) {
	s, _ := NewStream()
	defer s.Close()
	buf := voice(480)
	b.SetBytes(480 * 4)
	for i := 0; i < b.N; i++ {
		s.Process(buf, true, 1)
	}
}
