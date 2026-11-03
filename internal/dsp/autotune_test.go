package dsp

import (
	"math"
	"testing"
)

// tone — «голос» с гармониками на частоте f.
func tone(f float64, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		t := float64(i) / 48000
		v := 0.0
		for k := 1; k <= 6; k++ {
			v += math.Sin(2*math.Pi*f*float64(k)*t) / float64(k)
		}
		out[i] = float32(0.3 * v)
	}
	return out
}

// pitchOf — основной тон по автокорреляции (для проверки результата): первый пик, почти равный лучшему
// (кратные периоды при дробном периоде бывают чуть «лучше»).
func pitchOf(b []float32) float64 {
	lo, hi := 48000/1000, 48000/60
	r := make([]float64, hi+2)
	best := -1.0
	for lag := lo; lag <= hi+1; lag++ {
		c, e1, e2 := 0.0, 0.0, 0.0
		for i := 0; i+lag < len(b); i++ {
			c += float64(b[i]) * float64(b[i+lag])
			e1 += float64(b[i]) * float64(b[i])
			e2 += float64(b[i+lag]) * float64(b[i+lag])
		}
		r[lag] = c / math.Sqrt(e1*e2+1e-12)
		best = math.Max(best, r[lag])
	}
	for lag := lo + 1; lag <= hi; lag++ {
		if r[lag] >= 0.95*best && r[lag] >= r[lag-1] && r[lag] >= r[lag+1] {
			ft := float64(lag)
			if den := r[lag-1] + r[lag+1] - 2*r[lag]; den < 0 {
				ft += 0.5 * (r[lag-1] - r[lag+1]) / den
			}
			return 48000 / ft
		}
	}
	return 0
}

func semisBetween(a, b float64) float64 { return 12 * math.Log2(a/b) }

func runTune(t *testing.T, at *AutoTune, f float64) float64 {
	t.Helper()
	b := tone(f, 48000)
	for i := 0; i < len(b); i += 480 {
		at.Process(b[i : i+480])
	}
	return pitchOf(b[24000:28800])
}

func TestAutoTuneSnapsToNearestNote(t *testing.T) {
	cases := []struct {
		in, want float64
		scale    string
	}{
		{452, 440, "chromatic"},    // ля + 0,47 пт → ля
		{230, 233.08, "chromatic"}, // чуть ниже си-бемоль малой октавы
		{458, 440, "major"},        // до мажор: ля-диез нельзя → ля
		{128, 130.81, "major"},     // до малой октавы
	}
	for _, c := range cases {
		got := runTune(t, NewAutoTune(0, c.scale, 0, 0, 1, 48000), c.in)
		if d := math.Abs(semisBetween(got, c.want)); d > 0.2 {
			t.Errorf("%s %.0f Гц: получилось %.1f Гц, ждали %.1f (ошибка %.2f пт)", c.scale, c.in, got, c.want, d)
		}
	}
}

func TestAutoTuneShift(t *testing.T) {
	got := runTune(t, NewAutoTune(0, "chromatic", 0, 12, 1, 48000), 220)
	if d := math.Abs(semisBetween(got, 440)); d > 0.25 {
		t.Fatalf("сдвиг на октаву: %.1f Гц вместо 440", got)
	}
}

func TestAutoTuneSilenceStable(t *testing.T) {
	at := NewAutoTune(0, "minor", 0.3, 0, 1, 48000)
	b := make([]float32, 48000)
	at.Process(b)
	for _, x := range b {
		if x != 0 {
			t.Fatal("тишина должна оставаться тишиной")
		}
	}
}

func BenchmarkAutoTune(b *testing.B) {
	at := NewAutoTune(0, "chromatic", 0.2, 0, 1, 48000)
	buf := tone(200, 480)
	b.SetBytes(480 * 4)
	for i := 0; i < b.N; i++ {
		at.Process(buf)
	}
}
