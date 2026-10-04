package audio

import "testing"

func TestVizSpectrumPeak(t *testing.T) {
	v := NewViz(2048, 256, 96)
	copy(v.Samples(), Tone(1000, 2048.0/48000, 0.5, 48000))
	out := make([]byte, 256*2+96+1)
	if n := v.Frame(out); n != len(out) {
		t.Fatal(n)
	}
	spec := out[512 : 512+96]
	best := 0
	for i := range spec {
		if spec[i] > spec[best] {
			best = i
		}
	}
	want := 0
	for b := 0; b < 96; b++ {
		if v.edges[b] <= 43 && 43 < v.edges[b+1] { // бин 1 кГц
			want = b
		}
	}
	if best < want-1 || best > want+1 || spec[best] < 200 {
		t.Fatalf("пик в полосе %d (%d)", best, spec[best])
	}
	if out[len(out)-1] < 100 {
		t.Fatal("пиковый уровень")
	}
}
