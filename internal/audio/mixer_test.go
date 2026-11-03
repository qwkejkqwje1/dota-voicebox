package audio

import "testing"

func TestMixerModes(t *testing.T) {
	var m Mixer
	c := make([]float32, 2000)
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
	m.Add("a", c, 1, "interrupt") // старый затухает за 8 мс, новый — сначала
	if p := m.Playing(); len(p) != 1 || p[0].Pos != 0 {
		t.Fatalf("playing: %+v", p)
	}
	out = make([]float32, 1000)
	m.Render(out)
	if out[0] <= 0.5 || out[0] > 1 || out[fadeRestart+10] != 0.5 || !m.Active() {
		t.Fatalf("interrupt: %v %v", out[0], out[fadeRestart+10])
	}
	m.Render(make([]float32, 1000))
	if m.Active() {
		t.Fatal("звук должен закончиться")
	}
}

func TestMixerFadeStop(t *testing.T) {
	var m Mixer
	c := make([]float32, 48000)
	for i := range c {
		c[i] = 1
	}
	m.Add("x", c, 0.5, "toggle")
	if !m.Has("x") || m.Has("y") {
		t.Fatal("Has")
	}
	m.Stop("x")
	if m.Has("x") || !m.Active() {
		t.Fatal("после Stop звук затухает, но уже не «играет»")
	}
	out := make([]float32, fadeStop+100)
	m.Render(out)
	// плавное затухание без скачков
	for i := 1; i < fadeStop; i++ {
		if out[i] > out[i-1] || out[i-1]-out[i] > 0.01 {
			t.Fatalf("скачок на %d: %v → %v", i, out[i-1], out[i])
		}
	}
	if out[fadeStop+10] != 0 || m.Active() {
		t.Fatalf("хвост: %v active=%v", out[fadeStop+10], m.Active())
	}
	if got := MergePlaying([]Playing{{ID: "a", Pos: 1}}, []Playing{{ID: "a", Pos: 2}, {ID: "b"}}); len(got) != 2 || got[0].Pos != 2 {
		t.Fatalf("%+v", got)
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

func TestFindCableLegacy(t *testing.T) {
	p := []Device{{Name: "Динамики (Realtek)"}, {Name: "CABLE Input (VB-Audio Virtual Cable)"}}
	if FindCable(p) != "CABLE Input (VB-Audio Virtual Cable)" {
		t.Fatal("кабель не найден")
	}
	if !IsCableInput("CABLE Output (VB-Audio Virtual Cable)") || IsCableInput("Микрофон (USB)") {
		t.Fatal("IsCableInput")
	}
}
