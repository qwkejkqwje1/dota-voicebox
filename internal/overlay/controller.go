package overlay

import (
	"strings"
	"sync"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
)

// MenuPage — пунктов на странице (клавиши 1–9 и 0).
const MenuPage = 10

// Data — содержимое меню (его даёт приложение при каждом открытии и обновлении).
type Data struct {
	Sounds         []Item
	Effects        []Item  // пусто — вкладки «Голос» нет
	Layout         string  // list | strip | grid
	Position       string  // см. Positions
	X, Y           float64 // своё место (Position = custom)
	Monitor        string  // game | cursor | primary
	Opacity        float64 // непрозрачность фона
	Scale          float64
	TimeoutSec     int
	CloseAfterPick bool

	// набор текста (Enter): поиск по всем звукам и эффектам, фраза для команды
	Search  []Item   // всё, что можно найти (Kind: sound | effect)
	Phrases []string // недавние фразы (новые первыми)
	CanSay  bool     // есть синтез речи — можно сказать набранное

	// «Читалка»: набранный текст читает голос персонажа
	Voices   []Item   // персонажи (ID, Emoji, Label — имя, Find — описание)
	Voice    string   // выбранный персонаж
	ReadHist []string // недавно прочитанное (новое первым)

	Empty string // текст пустой вкладки «Звуки» ("" — стандартный)
}

// Source — приложение со стороны меню.
type Source interface {
	MenuData() Data
	Pick(kind, id string) // kind: "sound" | "effect" | "say" (id — фраза) | "read[:голос]", "readme[:голос]" (id — текст)
	StopAll()
}

// Result — что сделать окну после ввода.
type Result struct {
	Handled bool   // клавиша наша (не отдавать игре)
	Redraw  bool   // перерисовать
	Close   bool   // спрятать окно
	Act     func() // действие приложения (запускать вне перехватчика клавиш)
}

// Коды клавиш Windows, которые меню обрабатывает.
const (
	VKBack     = 0x08
	VKTab      = 0x09
	VKReturn   = 0x0D
	VKEscape   = 0x1B
	VKSpace    = 0x20
	VKPageUp   = 0x21
	VKPageDown = 0x22
	VKLeft     = 0x25
	VKUp       = 0x26
	VKRight    = 0x27
	VKDown     = 0x28
)

// IsModifier — Shift/Ctrl/Alt/Win (не закрывают меню).
func IsModifier(vk uint32) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0x5B, 0x5C, 0x14:
		return true
	}
	return false
}

// MenuKey — цифра на основном ряду или на цифровом блоке → номер строки 0..9.
func MenuKey(vk uint32) (int, bool) {
	switch {
	case vk >= 0x31 && vk <= 0x39:
		return int(vk - 0x31), true
	case vk == 0x30:
		return 9, true
	case vk >= 0x61 && vk <= 0x69:
		return int(vk - 0x61), true
	case vk == 0x60:
		return 9, true
	}
	return 0, false
}

// Controller — состояние быстрого меню: вкладка, страница, выделение, клавиши. Без привязки к Windows.
type Controller struct {
	R   *Renderer
	Src Source

	mu        sync.Mutex
	visible   bool
	tab       int
	page      int
	hover     int
	data      Data
	lastInput time.Time

	typing  bool   // строка поиска открыта
	query   []rune // что набрано
	results []Item // найденное (hover — выбранная строка)

	reader bool // «Читалка»: строки — голоса или недавние фразы
	voice  int  // выбранный голос (индекс в data.Voices)
	vstart int  // первый показанный голос
}

// Open показывает меню с первой страницы звуков.
func (c *Controller) Open() {
	d := c.Src.MenuData()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.visible, c.tab, c.page, c.hover, c.data = true, 0, 0, -1, d
	c.typing, c.query, c.results, c.reader = false, nil, nil, false
	c.syncVoiceLocked()
	c.lastInput = time.Now()
}

func (c *Controller) Close() {
	c.mu.Lock()
	c.visible = false
	c.mu.Unlock()
}

func (c *Controller) Visible() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.visible
}

// Data — текущие данные (позиция, масштаб).
func (c *Controller) Data() Data {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data
}

// Refresh перечитывает данные (что играет, какой эффект). true — что-то изменилось.
func (c *Controller) Refresh() bool {
	d := c.Src.MenuData()
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := !sameItems(c.data.Sounds, d.Sounds) || !sameItems(c.data.Effects, d.Effects)
	if c.typing {
		changed = changed || !sameItems(c.data.Search, d.Search) || strings.Join(c.data.Phrases, "\n") != strings.Join(d.Phrases, "\n") ||
			!sameItems(c.data.Voices, d.Voices) || strings.Join(c.data.ReadHist, "\n") != strings.Join(d.ReadHist, "\n")
	}
	c.data = d
	if c.typing {
		c.searchLocked()
		return changed
	}
	c.clampLocked()
	return changed
}

func sameItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c *Controller) listLocked() []Item {
	if c.tab == 1 && len(c.data.Effects) > 0 {
		return c.data.Effects
	}
	return c.data.Sounds
}

func (c *Controller) pagesLocked() int {
	n := (len(c.listLocked()) + MenuPage - 1) / MenuPage
	if n < 1 {
		n = 1
	}
	return n
}

func (c *Controller) clampLocked() {
	if c.tab == 1 && len(c.data.Effects) == 0 {
		c.tab = 0
	}
	if p := c.pagesLocked(); c.page >= p {
		c.page = p - 1
	}
	if c.hover >= len(c.pageItemsLocked()) {
		c.hover = -1
	}
}

func (c *Controller) pageItemsLocked() []Item {
	l := c.listLocked()
	from := c.page * MenuPage
	if from >= len(l) {
		return nil
	}
	return l[from:min(from+MenuPage, len(l))]
}

// tilesLocked — полоска или сетка: стрелки ← → ходят по плиткам.
func (c *Controller) tilesLocked() bool {
	return c.data.Layout == LayoutStrip || c.data.Layout == LayoutGrid
}

// flipLocked — следующая/предыдущая страница по кругу.
func (c *Controller) flipLocked(next bool) {
	p := c.pagesLocked()
	if next {
		c.page = (c.page + 1) % p
	} else {
		c.page = (c.page - 1 + p) % p
	}
	c.hover = -1
}

// stepLocked — выделение на соседнюю плитку; с края — на другую страницу.
func (c *Controller) stepLocked(d int) {
	n := len(c.pageItemsLocked())
	if n == 0 {
		return
	}
	if c.hover < 0 {
		c.hover = 0
		if d < 0 {
			c.hover = n - 1
		}
		return
	}
	h, p := c.hover+d, c.pagesLocked()
	switch {
	case h >= n:
		if p > 1 {
			c.page = (c.page + 1) % p
		}
		c.hover = 0
	case h < 0:
		if p > 1 {
			c.page = (c.page - 1 + p) % p
		}
		c.hover = len(c.pageItemsLocked()) - 1
	default:
		c.hover = h
	}
}

// rowLocked — в сетке ↑ ↓: на ряд выше или ниже.
func (c *Controller) rowLocked(down bool) {
	n := len(c.pageItemsLocked())
	if n == 0 {
		return
	}
	if c.hover < 0 {
		c.hover = 0
		return
	}
	h := c.hover - grCols
	if down {
		h = c.hover + grCols
	}
	switch {
	case h >= 0 && h < n:
		c.hover = h
	case down && c.hover < grCols && n > grCols: // во втором ряду меньше плиток
		c.hover = n - 1
	}
}

// Expired — пора закрыть по таймауту бездействия (пока набирают текст — не раньше чем через 45 с).
func (c *Controller) Expired(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.data.TimeoutSec
	if c.typing && t > 0 {
		t = max(t, 45)
	}
	return c.visible && t > 0 && now.Sub(c.lastInput) > time.Duration(t)*time.Second
}

// Key обрабатывает нажатие клавиши, пока меню открыто.
func (c *Controller) Key(vk uint32) Result { return c.Input(KeyInput{VK: vk}) }

// Input — нажатие с символом и модификаторами (окно меню переводит клавишу в символ, пока открыт поиск).
func (c *Controller) Input(k KeyInput) Result {
	vk := k.VK
	c.mu.Lock()
	if !c.visible {
		c.mu.Unlock()
		return Result{}
	}
	c.lastInput = time.Now()
	if k.Alt && !IsModifier(vk) { // Alt+Tab и прочие системные сочетания: меню закрывается, клавиша — системе
		c.visible = false
		c.stopTypingLocked()
		c.mu.Unlock()
		return Result{Close: true}
	}
	if c.typing {
		defer c.mu.Unlock()
		return c.typeKeyLocked(k)
	}
	if i, ok := MenuKey(vk); ok {
		c.mu.Unlock()
		return c.pick(i)
	}
	defer c.mu.Unlock()
	switch vk {
	case VKEscape:
		c.visible = false
		return Result{Handled: true, Close: true}
	case VKTab:
		if len(c.data.Effects) > 0 {
			c.tab = 1 - c.tab
			c.page, c.hover = 0, -1
		}
		return Result{Handled: true, Redraw: true}
	case VKPageUp, VKPageDown:
		c.flipLocked(vk == VKPageDown)
		return Result{Handled: true, Redraw: true}
	case VKLeft, VKRight:
		if c.tilesLocked() {
			d := 1
			if vk == VKLeft {
				d = -1
			}
			c.stepLocked(d)
		} else {
			c.flipLocked(vk == VKRight)
		}
		return Result{Handled: true, Redraw: true}
	case VKUp, VKDown:
		switch c.data.Layout {
		case LayoutStrip: // полоска: ↑ ↓ — страницы (как стрелки ▲ ▼ справа)
			c.flipLocked(vk == VKDown)
			return Result{Handled: true, Redraw: true}
		case LayoutGrid:
			c.rowLocked(vk == VKDown)
			return Result{Handled: true, Redraw: true}
		}
		n := len(c.pageItemsLocked())
		if n == 0 {
			return Result{Handled: true}
		}
		if vk == VKDown {
			c.hover = (c.hover + 1) % n
		} else {
			c.hover = (c.hover - 1 + n) % n
		}
		return Result{Handled: true, Redraw: true}
	case VKReturn:
		if c.hover < 0 { // ничего не выделено — строка поиска
			if c.canTypeLocked() {
				c.startTypingLocked()
				return Result{Handled: true, Redraw: true}
			}
			return Result{Handled: true}
		}
		h := c.hover
		c.mu.Unlock()
		r := c.pick(h)
		c.mu.Lock()
		return r
	case VKBack:
		return Result{Handled: true, Redraw: true, Act: c.Src.StopAll}
	}
	if IsModifier(vk) {
		return Result{}
	}
	// любая другая клавиша — игрок занялся игрой: закрываем меню, клавишу отдаём игре
	c.visible = false
	return Result{Close: true}
}

func (c *Controller) pick(i int) Result {
	c.mu.Lock()
	items := c.pageItemsLocked()
	if i < 0 || i >= len(items) {
		c.mu.Unlock()
		return Result{Handled: true}
	}
	it := items[i]
	kind := "sound"
	if c.tab == 1 && len(c.data.Effects) > 0 {
		kind = "effect"
	}
	closeAfter := c.data.CloseAfterPick
	if closeAfter {
		c.visible = false
	}
	c.hover = i
	c.mu.Unlock()
	src, id := c.Src, it.ID
	return Result{Handled: true, Redraw: !closeAfter, Close: closeAfter, Act: func() { src.Pick(kind, id) }}
}

// Wheel — прокрутка колёсиком над меню: листает страницы.
func (c *Controller) Wheel(delta int) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.visible || delta == 0 {
		return Result{}
	}
	c.lastInput = time.Now()
	if c.typing { // в «Читалке» колёсико выбирает голос, в поиске — ничего не листает
		if c.reader && strings.TrimSpace(string(c.query)) != "" {
			d := 1
			if delta > 0 {
				d = -1
			}
			c.stepVoiceLocked(d)
			return Result{Handled: true, Redraw: true}
		}
		return Result{Handled: true}
	}
	p := c.pagesLocked()
	if p < 2 {
		return Result{Handled: true}
	}
	if delta < 0 {
		c.page = min(c.page+1, p-1)
	} else {
		c.page = max(c.page-1, 0)
	}
	c.hover = -1
	return Result{Handled: true, Redraw: true}
}

// Touch — пользователь что-то делает с меню (тащит мышью): не закрывать по таймауту.
func (c *Controller) Touch() {
	c.mu.Lock()
	c.lastInput = time.Now()
	c.mu.Unlock()
}

// HoverAt — мышь над строкой i (-1 — ни над какой). true — нужно перерисовать.
func (c *Controller) HoverAt(i int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.typing && i < 0 { // в поиске выделение остаётся (Enter выбирает его)
		return false
	}
	if c.hover == i {
		return false
	}
	if c.typing && c.reader && i >= 0 && i < len(c.results) && c.results[i].Kind == "voice" {
		c.voice = c.vstart + i // голос под мышью — выбранный
	}
	c.hover = i
	c.lastInput = time.Now()
	return true
}

// Click — щелчок мышью по элементу из Layout.HitTest.
func (c *Controller) Click(kind string, i int) Result {
	c.mu.Lock()
	if c.typing {
		defer c.mu.Unlock()
		c.lastInput = time.Now()
		if kind == "row" {
			if c.reader {
				return c.readActivateLocked(i, false)
			}
			return c.activateLocked(i, false)
		}
		return Result{Handled: true}
	}
	c.mu.Unlock()
	switch kind {
	case "row":
		c.mu.Lock()
		c.lastInput = time.Now()
		c.mu.Unlock()
		return c.pick(i)
	case "tab":
		c.mu.Lock()
		defer c.mu.Unlock()
		c.lastInput = time.Now()
		if i != c.tab && (i == 0 || len(c.data.Effects) > 0) {
			c.tab, c.page, c.hover = i, 0, -1
			return Result{Handled: true, Redraw: true}
		}
	case "prev", "next":
		c.mu.Lock()
		defer c.mu.Unlock()
		c.lastInput = time.Now()
		if c.pagesLocked() > 1 {
			c.flipLocked(kind == "next")
			return Result{Handled: true, Redraw: true}
		}
	}
	return Result{Handled: true}
}

// View — что рисовать (scale — масштаб монитора; умножается на масштаб из настроек).
func (c *Controller) View(scale float64) MenuView {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.data.Scale
	if s <= 0 {
		s = 1
	}
	if c.typing {
		return c.typingViewLocked(scale * s)
	}
	v := MenuView{Layout: c.data.Layout, Tab: c.tab, Page: c.page, Pages: c.pagesLocked(), Hover: c.hover,
		Scale: scale * s, Opacity: c.data.Opacity}
	effects := len(c.data.Effects) > 0
	v.Tabs = []string{i18n.T("Звуки")}
	if effects {
		v.Tabs = append(v.Tabs, i18n.T("Голос"))
	}
	v.Footer = Footer(c.data.Layout, v.Pages, effects)
	if c.tilesLocked() { // одинаковый размер на всех вкладках и страницах — полоска не прыгает
		v.Slots = min(len(c.data.Sounds), MenuPage)
		if effects {
			v.Slots = max(v.Slots, min(len(c.data.Effects), MenuPage))
		}
	}
	items := c.pageItemsLocked()
	keys := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0"}
	for i, it := range items {
		it.Key = keys[i]
		v.Items = append(v.Items, it)
	}
	if c.tab == 0 && len(items) == 0 {
		v.Empty = c.data.Empty
		if v.Empty == "" {
			v.Empty = i18n.T("Звуков пока нет — добавьте их в VoiceBox")
		}
	}
	return v
}

// Footer — подсказки по клавишам внизу меню (у полоски подвала нет).
func Footer(layout string, pages int, effects bool) string {
	parts := []string{i18n.T("1–0 выбор")}
	if pages > 1 {
		k := "← →"
		if layout == LayoutGrid {
			k = "PgUp PgDn"
		}
		parts = append(parts, k+i18n.T(" стр."))
	}
	if effects {
		parts = append(parts, i18n.T("Tab голос"))
	}
	parts = append(parts, i18n.T("Enter поиск"))
	if pages <= 1 && !effects {
		parts = append(parts, i18n.T("Backspace стоп"))
	}
	return strings.Join(append(parts, "Esc"), "  ·  ")
}
