package audio

import (
	"math"
	"testing"
)

// производитель на 0.2% быстрее/медленнее потребителя: буфер должен оставаться
// около цели без опустошений после разгона.
func TestDriftCompensation(t *testing.T) {
	for _, skew := range []float64{1.002, 0.998, 1.0} {
		d := newDrift(960)
		out := make([]float32, 480)
		phase := 0.0
		acc := 0.0
		last := 0
		for blk := 0; blk < 20000; blk++ { // ~200 с звука
			acc += 480 * skew
			n := int(acc)
			acc -= float64(n)
			in := make([]float32, n)
			for i := range in {
				in[i] = float32(math.Sin(phase))
				phase += 2 * math.Pi * 440 / 48000
			}
			d.Write(in)
			last = d.q.Avail()
			d.Pull(out)
		}
		if u := d.Under.Load(); u > 1 {
			t.Fatalf("skew %v: %d опустошений", skew, u)
		}
		if a := last; a < 800 || a > 1150 {
			t.Fatalf("skew %v: заполнение %d", skew, a)
		}
	}
}
