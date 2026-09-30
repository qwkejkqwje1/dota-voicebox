package dsp

import (
	"math"
	"testing"
)

func TestPresetsStable(t *testing.T) {
	const sr = 48000
	for name, specs := range BuiltinPresets {
		c, err := BuildChain(name, specs, sr)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]float32, 480)
		peak := float32(0)
		for blk := 0; blk < 200; blk++ { // 2 секунды: голос 440 Гц + тишина
			for i := range buf {
				n := blk*len(buf) + i
				if blk < 100 {
					buf[i] = float32(0.3 * math.Sin(2*math.Pi*440*float64(n)/sr))
				} else {
					buf[i] = 0
				}
			}
			c.Process(buf)
			for _, x := range buf {
				if x != x {
					t.Fatalf("%s: NaN", name)
				}
				if a := float32(math.Abs(float64(x))); a > peak {
					peak = a
				}
			}
		}
		if peak > 1.5 {
			t.Errorf("%s: пик %.2f", name, peak)
		}
		if name != "clean" && peak < 0.05 {
			t.Errorf("%s: слишком тихо %.3f", name, peak)
		}
	}
}
