package dsp

import (
	"math"
	"os"
	"testing"
)

func TestAmbiencesLevelAndSafety(t *testing.T) {
	for _, a := range Ambiences {
		g := NewAmbience(a.ID, 48000)
		if g == nil {
			t.Fatalf("%s: нет генератора", a.ID)
		}
		buf := make([]float32, 48000*12)
		for i := 0; i < len(buf); i += 480 {
			g.Render(buf[i : i+480])
		}
		e, pk := 0.0, 0.0
		for _, x := range buf {
			if x != x {
				t.Fatalf("%s: NaN", a.ID)
			}
			e += float64(x) * float64(x)
			pk = math.Max(pk, math.Abs(float64(x)))
		}
		rms := math.Sqrt(e / float64(len(buf)))
		t.Logf("%-6s rms %.3f peak %.2f", a.ID, rms, pk)
		if rms < 0.05 || rms > 0.2 {
			t.Errorf("%s: громкость %.3f вне 0,05…0,2", a.ID, rms)
		}
		if pk > 2 { // при обычной громкости фона (30%) — не громче голоса
			t.Errorf("%s: пики %.2f", a.ID, pk)
		}
		if p := os.Getenv("AMB_DUMP"); p != "" {
			f, _ := os.Create(p + "/" + a.ID + ".f32")
			b := make([]byte, 4*len(buf))
			for i, x := range buf {
				u := math.Float32bits(x)
				b[4*i], b[4*i+1], b[4*i+2], b[4*i+3] = byte(u), byte(u>>8), byte(u>>16), byte(u>>24)
			}
			f.Write(b)
			f.Close()
		}
	}
	if NewAmbience("nope", 48000) != nil {
		t.Fatal("неизвестный фон")
	}
}

func TestLoopSeamless(t *testing.T) {
	data := make([]float32, 48000)
	for i := range data {
		data[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / 48000))
	}
	l := NewLoop(data, 48000)
	out := make([]float32, 48000*3)
	l.Render(out)
	for i := 1; i < len(out); i++ {
		if d := math.Abs(float64(out[i] - out[i-1])); d > 0.2 {
			t.Fatalf("скачок %.2f на стыке (отсчёт %d)", d, i)
		}
	}
	if NewLoop(data[:100], 48000) != nil {
		t.Fatal("слишком короткий звук не годится для фона")
	}
}

func BenchmarkAmbiences(b *testing.B) {
	gens := []Ambience{}
	for _, a := range Ambiences {
		gens = append(gens, NewAmbience(a.ID, 48000))
	}
	buf := make([]float32, 480)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gens[i%len(gens)].Render(buf)
	}
}
