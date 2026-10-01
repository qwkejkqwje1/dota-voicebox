package audio

import "testing"

func TestMixerModes(t *testing.T) {
	var m Mixer
	c := make([]float32, 100)
	for i := range c {
		c[i] = 0.5
	}
	m.Add("a", c, 1, "")
	m.Add("a", c, 1, "ignore") // не должен добавиться
	out := make([]float32, 60)
	m.Render(out)
	if out[0] != 0.5 {
		t.Fatalf("ignore: %v", out[0])
	}
	m.Add("a", c, 1, "interrupt") // заменяет
	out = make([]float32, 100)
	m.Render(out)
	if !(out[99] == 0.5) || m.Active() {
		t.Fatalf("interrupt: %v active=%v", out[99], m.Active())
	}
}

func TestRing(t *testing.T) {
	q := newRing(4)
	q.Write([]float32{1, 2, 3, 4, 5, 6})
	out := make([]float32, 5)
	q.ReadAdd(out)
	if out[0] != 3 || out[3] != 6 || out[4] != 0 {
		t.Fatal(out)
	}
}

func TestFindCable(t *testing.T) {
	p := []Device{{Name: "Динамики (Realtek)"}, {Name: "CABLE Input (VB-Audio Virtual Cable)"}}
	if FindCable(p) != "CABLE Input (VB-Audio Virtual Cable)" {
		t.Fatal("кабель не найден")
	}
	if !IsCableInput("CABLE Output (VB-Audio Virtual Cable)") || IsCableInput("Микрофон (USB)") {
		t.Fatal("IsCableInput")
	}
}
