package timers

import "testing"

func run(e *Engine, from, to int) []Fire {
	var all []Fire
	for t := from; t <= to; t++ {
		all = append(all, e.Update(t)...)
	}
	return all
}

func TestRunes(t *testing.T) {
	e := New([]Timer{
		{ID: "wisdom", Sound: "w", Start: 420, Every: 420, WarnBefore: 20},
		{ID: "mid6", Sound: "m", At: []int{360}, WarnBefore: 15},
		{ID: "power", Sound: "p", Start: 360, Every: 120, Skip: []int{360}, WarnBefore: 10},
	})
	f := run(e, -90, 900)
	want := map[string][]int{"wisdom": {420, 840}, "mid6": {360}, "power": {480, 600, 720, 840}}
	got := map[string][]int{}
	for _, x := range f {
		got[x.TimerID] = append(got[x.TimerID], x.EventAt)
	}
	for k, v := range want {
		if len(got[k]) != len(v) {
			t.Fatalf("%s: got %v want %v", k, got[k], v)
		}
		for i := range v {
			if got[k][i] != v[i] {
				t.Fatalf("%s: got %v want %v", k, got[k], v)
			}
		}
	}
}

func TestNoDuplicatesOnPauseAndJump(t *testing.T) {
	e := New([]Timer{{ID: "w", Sound: "w", Start: 420, Every: 420, WarnBefore: 20}})
	n := 0
	for i := 0; i < 5; i++ { // пауза: одно и то же время много раз
		n += len(e.Update(400))
	}
	if n != 1 {
		t.Fatalf("fired %d", n)
	}
	// прыжок далеко вперёд (переподключение) — старые события не спамим
	if f := e.Update(2000); len(f) != 0 {
		t.Fatalf("late fire %v", f)
	}
}

func TestOneShotAndReset(t *testing.T) {
	e := New(nil)
	e.Update(1000)
	e.AddOneShot("rosh_min", "r", 1480, 30)
	if f := run(e, 1001, 1460); len(f) != 1 || f[0].EventAt != 1480 {
		t.Fatalf("%v", f)
	}
	e.Update(10) // новая игра
	if f := run(e, 11, 2000); len(f) != 0 {
		t.Fatalf("oneshot survived reset: %v", f)
	}
}
