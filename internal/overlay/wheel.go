package overlay

import (
	"image"
	"math"
	"sync"
	"time"
)

// Колесо звуков (как колесо чата в Dota 2): зажали клавишу — вокруг курсора 8 секторов,
// повели мышь в сторону нужного — отпустили клавишу. Быстро нажали и отпустили — колесо
// остаётся, выбор щелчком или цифрой. Без привязки к Windows.

// WheelSlots — секторов в колесе (по часовой стрелке от верхнего).
const WheelSlots = 8

// Время и расстояния колеса.
const (
	WheelTap     = 280 * time.Millisecond // отпустили быстрее и мышь не двигали — колесо «залипает»
	WheelIdle    = 8 * time.Second        // «залипшее» колесо закрывается само после бездействия
	WheelHoldMax = 20 * time.Second       // страховка: отпускание клавиши так и не пришло
)

// WheelItem — сектор колеса.
type WheelItem struct {
	Act     string // действие: sound:<id> | preset:<id> | stop | mic_mute; "" — пусто
	Emoji   string
	Label   string
	Color   string
	Active  bool // играет / включён
	Pending bool // ещё готовится
}

// WheelData — содержимое и настройки (их даёт приложение при каждом открытии).
type WheelData struct {
	Items   []WheelItem // до 8, по часовой стрелке от верхнего
	Center  string      // cursor — у курсора | screen — по центру экрана
	Scale   float64
	Opacity float64
}

// WheelSource — приложение со стороны колеса.
type WheelSource interface {
	WheelData() WheelData
	WheelPick(act string)
}

// WheelView — что рисовать.
type WheelView struct {
	Items     []WheelItem // ровно 8
	Hover     int         // выбранный сектор (-1 — нет)
	Sticky    bool        // выбор щелчком (клавишу уже отпустили)
	Pointer   float64     // куда показывает мышь: радианы от верха по часовой стрелке
	PointerOn bool
	Scale     float64
	Opacity   float64
	Rev       int // меняется вместе со всем, кроме стрелки (для кэша картинки)
}

// WheelCtl — состояние колеса: открыто ли, какой сектор выбран, держат ли клавишу.
type WheelCtl struct {
	Src WheelSource

	mu        sync.Mutex
	visible   bool
	sticky    bool
	vk        uint32      // клавиша колеса (0 — открыто без клавиши)
	origin    image.Point // курсор при открытии: от него считаем направление, пока клавишу держат
	center    image.Point // центр колеса на экране
	scale     float64     // масштаб монитора × размер из настроек
	hover     int
	pointer   float64
	pointerOn bool
	data      WheelData
	items     []WheelItem
	opened    time.Time
	last      time.Time
	lastP     image.Point
	rev       int
}

// Load берёт у приложения содержимое и настройки колеса (до Open: от них зависят размер и место).
func (c *WheelCtl) Load() WheelData { return c.Src.WheelData() }

// Open показывает колесо. vk — клавиша, которую держат (0 — сразу выбор щелчком);
// held — держат ли её ещё (быстрое нажатие могло закончиться раньше, чем открылось окно).
func (c *WheelCtl) Open(d WheelData, vk uint32, held bool, origin, center image.Point, scale float64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := make([]WheelItem, WheelSlots)
	copy(items, d.Items)
	if scale <= 0 {
		scale = 1
	}
	c.visible, c.vk, c.origin, c.center, c.scale = true, vk, origin, center, scale
	c.sticky = vk == 0 || !held
	c.data, c.items, c.hover, c.pointerOn = d, items, -1, false
	c.opened, c.last, c.lastP = now, now, origin
	c.rev++
	c.moveLocked(origin, now)
}

func (c *WheelCtl) Close() {
	c.mu.Lock()
	c.visible = false
	c.mu.Unlock()
}

func (c *WheelCtl) Visible() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.visible
}

// Sticky — клавишу уже отпустили, выбор щелчком.
func (c *WheelCtl) Sticky() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sticky
}

// VK — клавиша колеса (0 — открыто без клавиши).
func (c *WheelCtl) VK() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.vk
}

// Геометрия колеса, логические пиксели.
const (
	whRin     = 66.0  // внутренний радиус секторов
	whRout    = 178.0 // внешний
	whGap     = 6.0   // зазор между секторами (постоянной ширины)
	whPop     = 7.0   // выбранный сектор выдвигается наружу
	whMargin  = 22.0  // поле под тень
	whCenterR = 56.0  // круг в середине
	whDead    = 26.0  // пока держат клавишу: ближе к началу — ничего не выбрано
	whReach   = 16.0  // «залипшее» колесо: сектор под курсором ещё чуть дальше края
)

// WheelSize — сторона квадратной картинки колеса, px.
func WheelSize(scale float64) int {
	if scale <= 0 {
		scale = 1
	}
	return int(2*(whRout+whPop+whMargin)*scale + 0.5)
}

// WheelRadius — радиус самого колеса без тени, px (чтобы не выходить за экран).
func WheelRadius(scale float64) int {
	if scale <= 0 {
		scale = 1
	}
	return int((whRout+whPop)*scale + 0.5)
}

// slotAt — сектор по направлению (радианы от верха по часовой стрелке).
func slotAt(a float64) int {
	step := 2 * math.Pi / WheelSlots
	i := int(math.Floor((a + step/2) / step))
	return ((i % WheelSlots) + WheelSlots) % WheelSlots
}

// direction — угол от верха по часовой стрелке (ось y экрана смотрит вниз).
func direction(dx, dy float64) float64 {
	a := math.Atan2(dx, -dy)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
}

// moveLocked пересчитывает выбор. true — изменился сектор или заметно сдвинулась стрелка.
func (c *WheelCtl) moveLocked(p image.Point, now time.Time) bool {
	ref, lo, hi := c.origin, whDead*c.scale, math.Inf(1)
	if c.sticky { // клавишу отпустили: выбор — сектор прямо под курсором
		ref, lo, hi = c.center, whCenterR*c.scale, (whRout+whPop+whReach)*c.scale
	}
	dx, dy := float64(p.X-ref.X), float64(p.Y-ref.Y)
	dist := math.Hypot(dx, dy)
	hover, on, ptr := -1, false, c.pointer
	if dist >= lo {
		on, ptr = true, direction(dx, dy)
		if dist <= hi {
			if i := slotAt(ptr); c.items[i].Act != "" {
				hover = i
			}
		}
	}
	if p != c.lastP {
		c.lastP, c.last = p, now
	}
	changed := hover != c.hover || on != c.pointerOn
	if on && !changed {
		d := math.Abs(ptr - c.pointer)
		changed = math.Min(d, 2*math.Pi-d) > 1.5*math.Pi/180
	}
	if hover != c.hover {
		c.rev++
	}
	if changed {
		c.hover, c.pointerOn, c.pointer = hover, on, ptr
	}
	return changed
}

// Move — курсор сдвинулся. true — перерисовать.
func (c *WheelCtl) Move(p image.Point, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.visible && c.moveLocked(p, now)
}

func (c *WheelCtl) pickLocked(i int) Result {
	if i < 0 || i >= len(c.items) || c.items[i].Act == "" {
		return Result{Handled: true}
	}
	c.visible = false
	src, act := c.Src, c.items[i].Act
	return Result{Handled: true, Close: true, Act: func() { src.WheelPick(act) }}
}

// releaseLocked — отпустили клавишу колеса.
func (c *WheelCtl) releaseLocked(now time.Time) Result {
	if c.sticky {
		return Result{}
	}
	if c.hover >= 0 {
		r := c.pickLocked(c.hover)
		r.Handled = false
		return r
	}
	if now.Sub(c.opened) < WheelTap { // быстро нажали и отпустили: оставить колесо, выбор щелчком
		c.sticky, c.last = true, now
		c.rev++
		c.moveLocked(c.lastP, now)
		return Result{Redraw: true}
	}
	c.visible = false // отпустили в середине — передумали
	return Result{Close: true}
}

// Release — отпустили клавишу колеса.
func (c *WheelCtl) Release(now time.Time) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.visible {
		return Result{}
	}
	return c.releaseLocked(now)
}

// WheelDigit — цифра 1–8 на основном ряду или цифровом блоке → сектор 0..7.
func WheelDigit(vk uint32) (int, bool) {
	switch {
	case vk >= 0x31 && vk <= 0x38:
		return int(vk - 0x31), true
	case vk >= 0x61 && vk <= 0x68:
		return int(vk - 0x61), true
	}
	return 0, false
}

// Key — клавиатура, пока колесо открыто. Handled — клавишу съесть (не отдавать игре).
// Отпускание клавиши колеса — выбор (сама клавиша игре отдаётся: её нажатие игра и так не видела).
func (c *WheelCtl) Key(vk uint32, down bool, now time.Time) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.visible {
		return Result{}
	}
	if vk == c.vk && c.vk != 0 {
		if !down {
			return c.releaseLocked(now)
		}
		if c.sticky { // нажали ещё раз — закрыть
			c.visible = false
			return Result{Handled: true, Close: true}
		}
		return Result{Handled: true} // автоповтор, пока держат
	}
	if !down {
		return Result{}
	}
	c.last = now
	if i, ok := WheelDigit(vk); ok {
		return c.pickLocked(i)
	}
	if vk == VKEscape {
		c.visible = false
		return Result{Handled: true, Close: true}
	}
	if IsModifier(vk) || !c.sticky {
		return Result{}
	}
	c.visible = false // «залипшее» колесо: игрок занялся игрой — закрыть, клавишу отдать игре
	return Result{Close: true}
}

// Click — левая кнопка мыши: выбрать сектор (мимо — закрыть).
func (c *WheelCtl) Click(p image.Point, now time.Time) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.visible {
		return Result{}
	}
	c.moveLocked(p, now)
	if c.hover < 0 {
		c.visible = false
		return Result{Handled: true, Close: true}
	}
	return c.pickLocked(c.hover)
}

// Tick — таймер окна: курсор и удержание клавиши (held — клавиша колеса ещё нажата).
func (c *WheelCtl) Tick(p image.Point, held bool, now time.Time) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.visible {
		return Result{Close: true}
	}
	moved := c.moveLocked(p, now)
	if !c.sticky {
		if !held {
			r := c.releaseLocked(now)
			r.Redraw = r.Redraw || moved
			return r
		}
		if now.Sub(c.opened) > WheelHoldMax {
			c.visible = false
			return Result{Close: true}
		}
	} else if now.Sub(c.last) > WheelIdle {
		c.visible = false
		return Result{Close: true}
	}
	return Result{Redraw: moved}
}

// View — что рисовать.
func (c *WheelCtl) View() WheelView {
	c.mu.Lock()
	defer c.mu.Unlock()
	return WheelView{Items: append([]WheelItem(nil), c.items...), Hover: c.hover, Sticky: c.sticky,
		Pointer: c.pointer, PointerOn: c.pointerOn, Scale: c.scale, Opacity: c.data.Opacity, Rev: c.rev}
}
