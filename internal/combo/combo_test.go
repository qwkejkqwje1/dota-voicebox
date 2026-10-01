package combo

import (
	"testing"
	"time"
)

var t0 = time.Unix(1000, 0)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

func press(m *Matcher, vk uint32, ms int) []Fired {
	f := m.Feed(Event{VK: vk, Down: true, T: at(ms)})
	f = append(f, m.Feed(Event{VK: vk, Down: false, T: at(ms + 30)})...)
	return f
}

const num1, num2, ctrlL = 0x61, 0x62, 0xA2

func TestDoubleTap(t *testing.T) {
	m := NewMatcher()
	single, _ := m.AddCombo("Num1", 0)
	double, _ := m.AddCombo("Num1 Num1", 350*time.Millisecond)
	f := press(m, num1, 0)
	if len(f) != 1 || f[0].ID != single {
		t.Fatalf("первое нажатие: %v", f)
	}
	f = press(m, num1, 200)
	if len(f) != 2 {
		t.Fatalf("двойное: %v", f)
	}
	// медленно — не двойное
	f = press(m, num1, 2000)
	f = press(m, num1, 2500)
	for _, x := range f {
		if x.ID == double {
			t.Fatal("медленное нажатие засчитано как двойное")
		}
	}
	// тройное нажатие: 2-е = двойное, 3-е начинает заново
	m.Reset()
	n := 0
	for i := 0; i < 3; i++ {
		for _, x := range press(m, num1, 5000+i*150) {
			if x.ID == double {
				n++
			}
		}
	}
	if n != 1 {
		t.Fatalf("тройное: %d двойных", n)
	}
}

func TestModsChordAndAutorepeat(t *testing.T) {
	m := NewMatcher()
	ctrl, _ := m.AddCombo("Ctrl+Num1", 0)
	chord, _ := m.AddCombo("Num1+Num2", 0)
	m.Feed(Event{VK: ctrlL, Down: true, T: at(0)})
	f := m.Feed(Event{VK: num1, Down: true, T: at(10)})
	if len(f) != 1 || f[0].ID != ctrl {
		t.Fatalf("ctrl: %v", f)
	}
	if f := m.Feed(Event{VK: num1, Down: true, T: at(40)}); f != nil {
		t.Fatalf("автоповтор: %v", f)
	}
	m.Feed(Event{VK: ctrlL, Down: false, T: at(50)})
	f = m.Feed(Event{VK: num2, Down: true, T: at(60)})
	if len(f) != 1 || f[0].ID != chord {
		t.Fatalf("аккорд: %v", f)
	}
}

func TestSequenceWithModifierBetween(t *testing.T) {
	m := NewMatcher()
	id, _ := m.AddCombo("Num1 Ctrl+Num2", 500*time.Millisecond)
	press(m, num1, 0)
	m.Feed(Event{VK: ctrlL, Down: true, T: at(100)})
	f := m.Feed(Event{VK: num2, Down: true, T: at(150)})
	if len(f) != 1 || f[0].ID != id {
		t.Fatalf("%v", f)
	}
}

func TestHold(t *testing.T) {
	m := NewMatcher()
	id, _ := m.AddHold("Num1", 500*time.Millisecond)
	m.Feed(Event{VK: num1, Down: true, T: at(0)})
	if f := m.Tick(at(300)); len(f) != 0 {
		t.Fatal("рано")
	}
	if f := m.Tick(at(520)); len(f) != 1 || f[0].ID != id || f[0].Kind != KindHold {
		t.Fatalf("%v", f)
	}
	if f := m.Tick(at(900)); len(f) != 0 {
		t.Fatal("повтор удержания")
	}
	if f := m.Feed(Event{VK: num1, Down: false, T: at(1000)}); len(f) != 1 || f[0].Kind != KindRelease {
		t.Fatalf("отпускание: %v", f)
	}
	// короткое нажатие — без hold и без release
	m.Feed(Event{VK: num1, Down: true, T: at(2000)})
	if f := m.Feed(Event{VK: num1, Down: false, T: at(2100)}); len(f) != 0 {
		t.Fatalf("%v", f)
	}
}

func TestParseErrors(t *testing.T) {
	m := NewMatcher()
	if _, err := m.AddCombo("Num1 Хрен", 0); err == nil {
		t.Fatal("нет ошибки")
	}
	if _, err := m.AddHold("Num1 Num1", time.Second); err == nil {
		t.Fatal("нет ошибки")
	}
}

func TestExactModifiers(t *testing.T) {
	m := NewMatcher()
	plain, _ := m.AddCombo("Num1", 0)
	ctrl, _ := m.AddCombo("Ctrl+Num1", 0)
	t0 := time.Now()
	m.Feed(Event{VK: 0xA2, Down: true, T: t0})
	f := m.Feed(Event{VK: 0x61, Down: true, T: t0})
	if len(f) != 1 || f[0].ID != ctrl {
		t.Fatalf("ctrl: %+v", f)
	}
	m.Feed(Event{VK: 0x61, Down: false, T: t0})
	m.Feed(Event{VK: 0xA2, Down: false, T: t0})
	f = m.Feed(Event{VK: 0x61, Down: true, T: t0})
	if len(f) != 1 || f[0].ID != plain {
		t.Fatalf("plain: %+v", f)
	}
}
