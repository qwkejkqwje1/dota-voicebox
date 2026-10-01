// Package gameclock — надёжное игровое время Dota 2.
//
// GSI присылает clock_time целыми секундами и только когда что-то меняется,
// пакеты могут задерживаться или пропадать (alt-tab, лаги, переподключение).
// Clock восстанавливает плавное время между пакетами:
//   - момент смены секунды фиксируется по приходу пакета с новым значением;
//   - между пакетами время экстраполируется (до MaxExtrapolate);
//   - пауза замораживает время;
//   - время не «дёргается» назад из-за джиттера;
//   - если GSI нет — можно задать время вручную (горн 0:00 / синхронизация).
package gameclock

import (
	"math"
	"sync"
	"time"
)

const (
	SrcGSI      = "gsi"      // свежий пакет GSI
	SrcEstimate = "estimate" // пакеты задерживаются — экстраполяция
	SrcManual   = "manual"   // время задано вручную
	SrcIdle     = "idle"     // Dota подключена, но матч не идёт (драфт/меню)
	SrcNone     = "none"
)

type Snapshot struct {
	Seconds   float64 `json:"seconds"`
	Clock     int     `json:"clock"` // целые секунды (floor)
	OK        bool    `json:"ok"`
	Source    string  `json:"source"`
	PacketAge float64 `json:"packet_age"` // сек с последнего пакета GSI, -1 если не было
	Paused    bool    `json:"paused"`
	Rate      float64 `json:"rate"` // пакетов GSI в секунду
	MatchID   string  `json:"match_id,omitempty"`
}

type Clock struct {
	mu  sync.Mutex
	now func() time.Time

	// MaxExtrapolate — сколько продолжать отсчёт без пакетов GSI.
	MaxExtrapolate time.Duration
	// Fresh — пакет младше этого считается «живым».
	Fresh time.Duration

	has, running, paused bool
	clock                int
	base                 time.Time // момент, когда clock стал текущим значением
	lastPacket           time.Time
	matchID              string
	packets              []time.Time
	lastOut              float64
	lastOutMatch         string

	manual     bool
	manualZero time.Time
}

func New(now func() time.Time) *Clock {
	if now == nil {
		now = time.Now
	}
	return &Clock{now: now, MaxExtrapolate: 90 * time.Second, Fresh: 2500 * time.Millisecond}
}

// OnGSI — пакет GSI. running = матч идёт (pre-game или in-progress).
// Возвращает true, если началась новая игра (сменился матч или время ушло назад).
func (c *Clock) OnGSI(clock int, running, paused bool, matchID string) (newGame bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.now()
	c.lastPacket = t
	c.packets = append(c.packets, t)
	for len(c.packets) > 0 && t.Sub(c.packets[0]) > 5*time.Second {
		c.packets = c.packets[1:]
	}
	if !running {
		c.has, c.running, c.paused, c.clock, c.base, c.matchID = true, false, paused, clock, t, matchID
		return false
	}
	if c.has && (matchID != c.matchID || clock < c.clock-5) {
		newGame = true
		c.has = false
		c.lastOut = math.Inf(-1)
	}
	switch {
	case !c.has || !c.running:
		c.base = t
	case paused:
		// в паузе время стоит
	case c.paused && !paused:
		c.base = t // сняли паузу — отсчёт секунды заново
	case clock != c.clock:
		c.base = t // видим смену секунды — лучшая привязка фазы
	default:
		// то же значение: если мы уже «убежали» вперёд — время на самом деле не шло
		if t.Sub(c.base) >= time.Second {
			c.base = t.Add(-999 * time.Millisecond)
		}
	}
	c.has, c.running, c.paused, c.clock, c.matchID = true, true, paused, clock, matchID
	c.manual = false // GSI главнее ручного времени
	return newGame
}

// SetManual задаёт текущее игровое время вручную (например 0 в момент горна).
func (c *Clock) SetManual(sec float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manual = true
	c.manualZero = c.now().Add(-time.Duration(sec * float64(time.Second)))
	c.lastOut = math.Inf(-1)
}

// ClearManual — сбросить ручное время.
func (c *Clock) ClearManual() {
	c.mu.Lock()
	c.manual = false
	c.mu.Unlock()
}

// Reset — забыть всё (новая игра).
func (c *Clock) Reset() {
	c.mu.Lock()
	c.has, c.manual = false, false
	c.lastOut = math.Inf(-1)
	c.mu.Unlock()
}

func (c *Clock) Now() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.now()
	s := Snapshot{Source: SrcNone, PacketAge: -1, MatchID: c.matchID}
	if !c.lastPacket.IsZero() {
		s.PacketAge = t.Sub(c.lastPacket).Seconds()
		if n := len(c.packets); n > 1 {
			if span := t.Sub(c.packets[0]).Seconds(); span > 0 {
				s.Rate = math.Round(float64(n)/math.Max(span, 1)*10) / 10
			}
		}
	}
	age := t.Sub(c.lastPacket)
	switch {
	case c.has && c.running && age <= c.MaxExtrapolate:
		s.OK, s.Paused = true, c.paused
		s.Source = SrcGSI
		if age > c.Fresh {
			s.Source = SrcEstimate
		}
		s.Seconds = float64(c.clock)
		if !c.paused {
			el := t.Sub(c.base).Seconds()
			if age <= c.Fresh && el > 0.999 {
				// пакеты свежие, но новой секунды ещё не было — не убегаем вперёд больше чем на 1с
				el = math.Min(el, 1.2)
			}
			s.Seconds += math.Max(0, el)
		}
	case c.manual:
		s.OK, s.Source = true, SrcManual
		s.Seconds = t.Sub(c.manualZero).Seconds()
	case c.has && !c.running && age <= 40*time.Second:
		s.Source = SrcIdle
		s.Seconds = float64(c.clock)
	}
	if s.OK {
		// монотонность: мелкий откат из-за джиттера не показываем
		if s.Seconds < c.lastOut && s.Seconds > c.lastOut-2 {
			s.Seconds = c.lastOut
		}
		c.lastOut = s.Seconds
		s.Clock = int(math.Floor(s.Seconds))
	}
	return s
}
