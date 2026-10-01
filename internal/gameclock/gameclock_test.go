package gameclock

import (
	"testing"
	"time"
)

type fake struct{ t time.Time }

func (f *fake) now() time.Time      { return f.t }
func (f *fake) adv(d time.Duration) { f.t = f.t.Add(d) }

func TestExtrapolateAndPause(t *testing.T) {
	f := &fake{t: time.Unix(1000, 0)}
	c := New(f.now)
	if c.Now().OK {
		t.Fatal("no data must be !OK")
	}
	c.OnGSI(100, true, false, "m1")
	f.adv(500 * time.Millisecond)
	if s := c.Now(); s.Clock != 100 || s.Seconds < 100.49 || s.Source != SrcGSI {
		t.Fatalf("got %+v", s)
	}
	f.adv(500 * time.Millisecond)
	c.OnGSI(101, true, false, "m1")
	f.adv(250 * time.Millisecond)
	if s := c.Now(); s.Seconds < 101.24 || s.Seconds > 101.26 {
		t.Fatalf("phase lock %+v", s)
	}
	// пакеты пропали на 10 секунд — экстраполяция
	f.adv(10 * time.Second)
	if s := c.Now(); s.Clock != 111 || s.Source != SrcEstimate {
		t.Fatalf("estimate %+v", s)
	}
	// пакет с правдой: 111 → без отката
	c.OnGSI(111, true, false, "m1")
	if s := c.Now(); s.Clock != 111 {
		t.Fatalf("resync %+v", s)
	}
	// пауза
	c.OnGSI(112, true, true, "m1")
	f.adv(30 * time.Second)
	c.OnGSI(112, true, true, "m1")
	if s := c.Now(); s.Clock != 112 || !s.Paused {
		t.Fatalf("pause %+v", s)
	}
	c.OnGSI(112, true, false, "m1")
	f.adv(400 * time.Millisecond)
	if s := c.Now(); s.Clock != 112 {
		t.Fatalf("unpause %+v", s)
	}
	// слишком долго без пакетов — время потеряно
	f.adv(5 * time.Minute)
	if c.Now().OK {
		t.Fatal("must expire")
	}
}

func TestNoRunAheadWithFreshPackets(t *testing.T) {
	f := &fake{t: time.Unix(1000, 0)}
	c := New(f.now)
	c.OnGSI(50, true, false, "m")
	for i := 0; i < 10; i++ { // пакеты идут (меняется HP), но секунда «зависла»
		f.adv(300 * time.Millisecond)
		c.OnGSI(50, true, false, "m")
	}
	if s := c.Now(); s.Clock != 50 {
		t.Fatalf("run ahead %+v", s)
	}
}

func TestNewGameAndManual(t *testing.T) {
	f := &fake{t: time.Unix(1000, 0)}
	c := New(f.now)
	c.OnGSI(900, true, false, "a")
	if !c.OnGSI(-90, true, false, "b") {
		t.Fatal("new match expected")
	}
	if s := c.Now(); s.Clock != -90 {
		t.Fatalf("%+v", s)
	}
	c2 := New(f.now)
	c2.SetManual(0)
	f.adv(61500 * time.Millisecond)
	if s := c2.Now(); s.Clock != 61 || s.Source != SrcManual {
		t.Fatalf("manual %+v", s)
	}
	c2.OnGSI(10, false, false, "")
	if s := c2.Now(); s.Source != SrcManual {
		t.Fatalf("idle gsi must not kill manual %+v", s)
	}
}
