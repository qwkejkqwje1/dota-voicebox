package dsp

import (
	"math"
	"testing"
	"time"
)

const tsr = 48000.0

// voiceLike — «голос»: гармоники 140 Гц с огибающей слогов.
func voiceLike(n int, amp float64) []float32 {
	b := make([]float32, n)
	for i := range b {
		t := float64(i) / tsr
		env := 0.6 + 0.4*math.Sin(2*math.Pi*4*t)
		var v float64
		for h := 1.0; h <= 12; h++ {
			v += math.Sin(2*math.Pi*140*h*t) / h
		}
		b[i] = float32(amp * env * v / 2)
	}
	return b
}

func rms(b []float32) float64 {
	if len(b) == 0 {
		return 0
	}
	var s float64
	for _, x := range b {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s / float64(len(b)))
}

// goertzel — мощность частоты f в буфере.
func goertzel(b []float32, f float64) float64 {
	w := 2 * math.Pi * f / tsr
	c := 2 * math.Cos(w)
	var s1, s2 float64
	for _, x := range b {
		s := float64(x) + c*s1 - s2
		s2, s1 = s1, s
	}
	return s1*s1 + s2*s2 - c*s1*s2
}

func runChain(t *testing.T, specs []Spec, in []float32) []float32 {
	t.Helper()
	c, err := BuildChain("t", specs, tsr)
	if err != nil {
		t.Fatal(err)
	}
	out := append([]float32(nil), in...)
	for i := 0; i < len(out); i += 480 {
		c.Process(out[i:min(i+480, len(out))])
	}
	return out
}

func ms(v float64) int { return int(v * tsr / 1000) }

func TestRadioSilenceIsSilent(t *testing.T) {
	p, _ := FindPreset("radio")
	out := runChain(t, p.Specs(nil), make([]float32, ms(1000)))
	if r := rms(out); r > 1e-6 {
		t.Fatalf("рация шумит без передачи: rms %.6f", r)
	}
}

func TestRadioSquelchOpenAndTail(t *testing.T) {
	p, _ := FindPreset("radio")
	in := append(voiceLike(ms(1000), 0.2), make([]float32, ms(1500))...)
	out := runChain(t, p.Specs(nil), in)
	speech := rms(out[ms(200):ms(900)])
	if speech < 0.05 {
		t.Fatalf("голос слишком тихий: %.3f", speech)
	}
	// хвост шумоподавителя: после конца речи + удержание (~380 мс) — всплеск шума
	tail := rms(out[ms(1300):ms(1600)])
	if tail < 0.01 {
		t.Fatalf("нет хвоста «пшш» после фразы: rms %.4f", tail)
	}
	if end := rms(out[ms(2000):]); end > 1e-4 {
		t.Fatalf("после хвоста должна быть тишина: %.5f", end)
	}
}

func TestRadioIgnoresKeyboardClick(t *testing.T) {
	p, _ := FindPreset("radio")
	in := make([]float32, ms(1000))
	for k := 0; k < 5; k++ { // пять щелчков клавиатуры по 3 мс
		at := ms(100 + 150*float64(k))
		for i := 0; i < ms(3); i++ {
			in[at+i] = float32(0.6 * math.Exp(-float64(i)/30) * math.Sin(float64(i)))
		}
	}
	out := runChain(t, p.Specs(nil), in)
	if r := rms(out); r > 1e-4 {
		t.Fatalf("щелчки клавиатуры открыли рацию: rms %.5f", r)
	}
}

func TestRadioKeyedRogerBeep(t *testing.T) {
	p, _ := FindPreset("radio")
	specs := p.Specs(map[string]float64{"beep": 1, "noise": 0})
	c, _ := BuildChain("r", specs, tsr)
	out := make([]float32, 0, ms(1200))
	blk := make([]float32, 480)
	for i := 0; i < ms(1200); i += 480 {
		key := i < ms(500)
		c.SetKey(key, true)
		copy(blk, voiceLike(480, 0.1))
		if !key {
			clear(blk)
		}
		c.Process(blk)
		out = append(out, blk...)
	}
	// бип 1250 Гц: ~30 мс паузы после отпускания + 110 мс тона (с упреждением 18 мс)
	win := out[ms(560):ms(650)]
	if goertzel(win, 1250) < 10*goertzel(win, 700) {
		t.Fatalf("нет роджер-бипа 1250 Гц после отпускания тангенты")
	}
}

func TestRadioKeyedCarrierHiss(t *testing.T) {
	p, _ := FindPreset("radio")
	c, _ := BuildChain("r", p.Specs(nil), tsr)
	buf := make([]float32, 480)
	var sum float64
	for i := 0; i < 100; i++ {
		c.SetKey(true, true)
		clear(buf)
		c.Process(buf)
		if i > 20 {
			sum += rms(buf)
		}
	}
	if sum/79 < 1e-3 {
		t.Fatalf("при нажатой тангенте должно быть шипение эфира")
	}
}

func TestPitchShift(t *testing.T) {
	in := make([]float32, ms(500))
	for i := range in {
		in[i] = float32(0.3 * math.Sin(2*math.Pi*440*float64(i)/tsr))
	}
	up := runChain(t, []Spec{{Type: "pitch", Semitones: 12}}, in)[ms(100):]
	if goertzel(up, 880) < 4*goertzel(up, 440) {
		t.Fatal("+12 пт: нет октавы вверх")
	}
	down := runChain(t, []Spec{{Type: "pitch", Semitones: -12}}, in)[ms(100):]
	if goertzel(down, 220) < 4*goertzel(down, 440) {
		t.Fatal("-12 пт: нет октавы вниз")
	}
}

func TestReverbTail(t *testing.T) {
	in := make([]float32, ms(1500))
	in[0] = 1
	out := runChain(t, []Spec{{Type: "reverb", Room: 0.8, Wet: 0.5, Dry: 1}}, in)
	if rms(out[ms(100):ms(400)]) < 1e-4 {
		t.Fatal("у реверберации нет хвоста")
	}
	if rms(out[ms(1200):]) > rms(out[ms(100):ms(400)]) {
		t.Fatal("хвост реверберации не затухает")
	}
}

func TestMuLawQuantizes(t *testing.T) {
	in := make([]float32, 4800)
	for i := range in {
		in[i] = float32(math.Sin(float64(i) / 37))
	}
	out := runChain(t, []Spec{{Type: "mulaw"}}, in)
	uniq := map[float32]bool{}
	for _, x := range out {
		uniq[x] = true
		if x > 1 || x < -1 {
			t.Fatal("выход за пределы")
		}
	}
	if len(uniq) > 256 {
		t.Fatalf("μ-law: %d уровней, ожидалось ≤ 256", len(uniq))
	}
}

// Каждый пресет на крайних значениях ручек: без NaN, без перегруза, не тишина, быстро.
func TestCatalogExtremes(t *testing.T) {
	in := append(voiceLike(ms(800), 0.25), make([]float32, ms(400))...)
	for _, p := range Catalog {
		variants := []map[string]float64{nil, {}, {}}
		for _, k := range p.Knobs {
			variants[1][k.ID], variants[2][k.ID] = k.Min, k.Max
			if k.Kind == "toggle" {
				variants[1][k.ID], variants[2][k.ID] = 0, 1
			}
			if k.Kind == "choice" {
				variants[1][k.ID] = k.Options[0].Value
				variants[2][k.ID] = k.Options[len(k.Options)-1].Value
			}
		}
		for vi, v := range variants {
			start := time.Now()
			out := runChain(t, p.Specs(v), in)
			el := time.Since(start)
			var pk float64
			for _, x := range out {
				if x != x {
					t.Fatalf("%s/%d: NaN", p.ID, vi)
				}
				pk = math.Max(pk, math.Abs(float64(x)))
			}
			if pk > 1.2 {
				t.Errorf("%s/%d: пик %.2f", p.ID, vi, pk)
			}
			if p.ID != "clean" && pk < 0.02 {
				t.Errorf("%s/%d: слишком тихо (%.3f)", p.ID, vi, pk)
			}
			if el > 400*time.Millisecond { // 1.2 с звука; на практике ~10-30 мс
				t.Errorf("%s/%d: медленно %v", p.ID, vi, el)
			}
		}
	}
}

func TestPresetValues(t *testing.T) {
	p, _ := FindPreset("radio")
	v := p.Values(map[string]float64{"noise": 500, "beep": 77, "squelch": 5})
	if v["noise"] != 100 || v["beep"] != 0 || v["squelch"] != 1 || v["drive"] != 40 {
		t.Fatalf("неверная нормализация ручек: %v", v)
	}
	if len(PresetOrder) != len(Catalog) || len(BuiltinPresets) != len(Catalog) {
		t.Fatal("каталог не совпадает с BuiltinPresets/PresetOrder")
	}
	for _, id := range []string{"clean", "dead_inside", "dead_inside_max", "radio", "radio_broken", "megaphone", "robot"} {
		if _, ok := FindPreset(id); !ok {
			t.Fatalf("пропал старый пресет %s (совместимость конфигов)", id)
		}
	}
}
