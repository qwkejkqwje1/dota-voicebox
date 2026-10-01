package audio

import (
	"math/rand"
	"testing"
)

func TestAnalyzeAndTone(t *testing.T) {
	sr := 48000
	b := make([]float32, sr*2)
	r := rand.New(rand.NewSource(1))
	for i := range b {
		b[i] = float32(r.NormFloat64() * 0.001) // шум -60 dB
	}
	tone := Tone(1000, 0.5, 0.25, sr)
	copy(b[sr/2:], tone) // тон с 0.5 с
	st := Analyze(b, sr)
	if st.Silent || st.NoiseDB > -50 || st.SpeechDB < -20 || st.SuggestGain <= 0 {
		t.Fatalf("%+v", st)
	}
	on, lv := ToneOnset(b, sr, 1000)
	if on < 0.49 || on > 0.52 || lv < -16 || lv > -9 {
		t.Fatalf("onset %v level %v", on, lv)
	}
	// другой частоты (речь/шум) — не тон
	other := Tone(400, 0.5, 0.3, sr)
	if on, _ := ToneOnset(other, sr, 1000); on >= 0 {
		t.Fatalf("false tone at %v", on)
	}
	if s := Analyze(make([]float32, sr), sr); !s.Silent {
		t.Fatal("silence")
	}
}
