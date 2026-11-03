package audio

import (
	"math"
	"testing"
	"unsafe"

	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
)

func asBytes(f []float32) []byte { return unsafe.Slice((*byte)(unsafe.Pointer(&f[0])), len(f)*4) }

// pump прогоняет моно-сигнал микрофона через движок кусками по 441 отсчёт; возвращает эфир (моно).
func pump(e *Engine, mic []float32) []float32 {
	const n = 441
	out := make([]float32, 0, len(mic))
	o := make([]float32, 2*n)
	for i := 0; i+n <= len(mic); i += n {
		in := append([]float32(nil), mic[i:i+n]...)
		e.onDuplex(asBytes(o), asBytes(in), n)
		for k := 0; k < n; k++ {
			out = append(out, o[2*k])
		}
	}
	return out
}

func rms(b []float32) float64 {
	e := 0.0
	for _, x := range b {
		e += float64(x) * float64(x)
	}
	return math.Sqrt(e / float64(len(b)))
}

func whiteNoise(n int, amp float32) []float32 {
	out := make([]float32, n)
	s := uint32(1)
	for i := range out {
		s = s*1664525 + 1013904223
		out[i] = (float32(s>>9)/8388608 - 1) * amp
	}
	return out
}

func TestDenoiseInEngine(t *testing.T) {
	e := NewEngine()
	nz := whiteNoise(48000*2, 0.05)
	before := rms(pump(e, nz)[48000:])
	if err := e.SetDenoise(true, 1); err != nil {
		t.Fatal(err)
	}
	after := rms(pump(e, nz)[48000:])
	if db := 20 * math.Log10(after/before); db > -15 {
		t.Fatalf("шумоподавление в движке: только %.1f дБ", db)
	}
	e.SetDenoise(false, 1)
	if r := rms(pump(e, nz)[4800:]); math.Abs(r-before)/before > 0.05 {
		t.Fatalf("после выключения сигнал должен быть как раньше: %.4f vs %.4f", r, before)
	}
}

func TestBackgroundOnlyWhileTalking(t *testing.T) {
	e := NewEngine()
	e.SetBackground(dsp.NewAmbience("rain", 48000))
	e.SetBackgroundMix(0.5, true)
	silence := make([]float32, 48000)
	if r := rms(pump(e, silence)); r > 1e-4 {
		t.Fatalf("фон «пока говорю» звучит в тишине: %.4f", r)
	}
	tone := make([]float32, 48000)
	for i := range tone {
		tone[i] = float32(0.3 * math.Sin(2*math.Pi*200*float64(i)/48000))
	}
	withBG := pump(e, tone)
	e.SetBackground(nil)
	pump(e, silence) // старый фон уходит
	plain := pump(e, tone)
	diff := make([]float32, 20000)
	for i := range diff {
		diff[i] = withBG[24000+i] - plain[24000+i]
	}
	if r := rms(diff); r < 0.02 {
		t.Fatalf("фон не слышен, пока говорю: %.4f", r)
	}
	// режим «всегда»
	e.SetBackground(dsp.NewAmbience("sea", 48000))
	e.SetBackgroundMix(0.5, false)
	if r := rms(pump(e, silence)[24000:]); r < 0.01 {
		t.Fatalf("фон «всегда» не звучит: %.4f", r)
	}
	e.SetMicStrip(true, 0) // микрофон выключен — фон тоже (плавно уходит за 0,6 с)
	if r := rms(pump(e, silence)[36000:]); r > 1e-4 {
		t.Fatalf("микрофон выключен, а фон звучит: %.4f", r)
	}
}

func TestTimbreSwitchSmooth(t *testing.T) {
	e := NewEngine()
	tone := make([]float32, 48000)
	for i := range tone {
		tone[i] = float32(0.2 * math.Sin(2*math.Pi*220*float64(i)/48000))
	}
	pump(e, tone[:4410])
	c, err := dsp.BuildChain("timbre", dsp.TimbreSpecs([]float64{6, -3, 0, 4, 2}, 0.6), 48000)
	if err != nil {
		t.Fatal(err)
	}
	e.SetTimbre(c)
	out := pump(e, tone)
	for i := 1; i < len(out); i++ {
		if d := math.Abs(float64(out[i] - out[i-1])); d > 0.1 || out[i] != out[i] {
			t.Fatalf("щелчок при смене тембра: скачок %.3f на %d", d, i)
		}
	}
}
