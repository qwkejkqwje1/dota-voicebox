package overlay

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/image/vector"
)

// Набор текста прямо в меню поверх игры: Enter (или своя клавиша) — строка поиска. Набранное ищется среди
// всех звуков и эффектов; последняя строка — «Сказать команде: …» (фраза озвучивается голосом Windows).

const (
	searchRows = 7   // результатов поиска (плюс строка «Сказать команде»)
	maxQuery   = 200 // символов в строке
)

// KeyInput — нажатие для меню: код клавиши, символ в раскладке игрока и модификаторы.
type KeyInput struct {
	VK   uint32
	Text string // что печатает клавиша ("" — не символ)
	Ctrl bool   // зажат Ctrl (без Alt)
	Alt  bool   // зажат Alt (без Ctrl; Ctrl+Alt — это AltGr, печатает символы)
}

// Typing — открыта ли строка поиска (окну нужно знать, переводить ли клавиши в символы).
func (c *Controller) Typing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.visible && c.typing
}

// StartTyping открывает строку поиска (меню должно быть открыто).
func (c *Controller) StartTyping() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.visible {
		c.startTypingLocked()
	}
}

func (c *Controller) canTypeLocked() bool { return len(c.data.Search) > 0 || c.data.CanSay }

func (c *Controller) startTypingLocked() {
	c.typing, c.query = true, nil
	c.lastInput = time.Now()
	c.searchLocked()
}

func (c *Controller) stopTypingLocked() {
	c.typing, c.query, c.results, c.hover = false, nil, nil, -1
}

// searchLocked пересчитывает результаты для набранного текста.
func (c *Controller) searchLocked() {
	q := strings.TrimSpace(string(c.query))
	var out []Item
	if q == "" {
		for _, p := range c.data.Phrases {
			if len(out) == searchRows {
				break
			}
			out = append(out, Item{Kind: "say", ID: p, Emoji: "🗣️", Label: p, Color: "#8b5cf6", Hint: "недавняя фраза"})
		}
	} else {
		out = Find(c.data.Search, q, searchRows)
		if c.data.CanSay {
			out = append(out, Item{Kind: "say", ID: q, Emoji: "🗣️", Label: "Сказать команде: «" + q + "»", Color: "#8b5cf6"})
		}
	}
	c.results = out
	switch {
	case len(out) == 0:
		c.hover = -1
	case c.hover < 0 || c.hover >= len(out):
		c.hover = 0
	}
}

// typeKeyLocked — клавиша в режиме набора. Все клавиши, кроме модификаторов, остаются в меню (игра их не получает).
func (c *Controller) typeKeyLocked(k KeyInput) Result {
	switch k.VK {
	case VKEscape:
		c.stopTypingLocked()
		return Result{Handled: true, Redraw: true}
	case VKReturn:
		return c.activateLocked(c.hover, k.Ctrl)
	case VKBack:
		if k.Ctrl {
			c.query = nil
		} else if n := len(c.query); n > 0 {
			c.query = c.query[:n-1]
		}
		c.hover = 0
		c.searchLocked()
		return Result{Handled: true, Redraw: true}
	case VKUp, VKDown:
		if n := len(c.results); n > 0 {
			switch {
			case c.hover < 0:
				c.hover = 0
			case k.VK == VKDown:
				c.hover = (c.hover + 1) % n
			default:
				c.hover = (c.hover - 1 + n) % n
			}
		}
		return Result{Handled: true, Redraw: true}
	}
	if IsModifier(k.VK) {
		return Result{}
	}
	if t := printable(k.Text); t != "" && !k.Ctrl {
		if r := []rune(t); len(c.query)+len(r) <= maxQuery {
			c.query = append(c.query, r...)
		}
		c.hover = 0
		c.searchLocked()
		return Result{Handled: true, Redraw: true}
	}
	return Result{Handled: true}
}

// activateLocked — Enter в режиме набора: выбранная строка; say — сказать набранное, что бы ни было выбрано.
func (c *Controller) activateLocked(i int, say bool) Result {
	q := strings.TrimSpace(string(c.query))
	var kind, id string
	switch {
	case say && q != "" && c.data.CanSay:
		kind, id = "say", q
	case i >= 0 && i < len(c.results):
		kind, id = c.results[i].Kind, c.results[i].ID
	default:
		return Result{Handled: true}
	}
	if kind == "" {
		kind = "sound"
	}
	// после выбора строка поиска закрывается: клавиши снова идут в игру
	c.stopTypingLocked()
	closeAfter := c.data.CloseAfterPick || kind == "say"
	if closeAfter {
		c.visible = false
	}
	src := c.Src
	return Result{Handled: true, Redraw: !closeAfter, Close: closeAfter, Act: func() { src.Pick(kind, id) }}
}

func printable(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// typingViewLocked — меню в режиме набора: всегда список, вместо вкладок — строка поиска.
func (c *Controller) typingViewLocked(scale float64) MenuView {
	v := MenuView{Layout: LayoutList, Typing: true, Query: string(c.query), Hover: c.hover, Pages: 1, Scale: scale,
		Opacity: c.data.Opacity, Hint: "Звук, эффект или фраза для команды"}
	if !c.data.CanSay {
		v.Hint = "Название звука или эффекта"
	}
	for i, it := range c.results {
		it.Key = ""
		if i == c.hover {
			it.Hint = "Enter"
		}
		v.Items = append(v.Items, it)
	}
	if len(v.Items) == 0 {
		switch {
		case len(c.query) > 0:
			v.Empty = "Ничего не нашлось"
		case c.data.CanSay:
			v.Empty = "Напишите — найду звук или скажу команде"
		default:
			v.Empty = "Напишите название звука"
		}
	}
	v.Footer = "↑↓ выбор  ·  Enter  ·  Ctrl+Enter сказать  ·  Esc назад"
	if !c.data.CanSay {
		v.Footer = "↑↓ выбор  ·  Enter  ·  Esc назад"
	}
	return v
}

// ---------- поиск ----------

// Find — пункты, в названии (или словах для поиска) которых есть все слова запроса; сначала те, что
// начинаются с запроса. Если ничего не нашлось — пробует запрос в другой раскладке («ghbdtn» → «привет»).
func Find(items []Item, query string, limit int) []Item {
	out := find(items, query, limit)
	if len(out) == 0 {
		if alt := swapLayout(query); alt != query {
			out = find(items, alt, limit)
		}
	}
	return out
}

func normText(s string) string { return strings.ReplaceAll(strings.ToLower(s), "ё", "е") }

func find(items []Item, query string, limit int) []Item {
	q := normText(strings.TrimSpace(query))
	words := strings.Fields(q)
	if len(words) == 0 {
		return nil
	}
	type hit struct {
		it    Item
		score int
	}
	var hits []hit
	for _, it := range items {
		label := normText(it.Label)
		hay := label + " " + normText(it.Find)
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		score := 3
		switch {
		case strings.HasPrefix(label, q):
			score = 0
		case wordPrefix(label, words[0]):
			score = 1
		case strings.Contains(label, words[0]):
			score = 2
		}
		hits = append(hits, hit{it, score})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score < hits[j].score })
	var out []Item
	for _, h := range hits {
		if len(out) == limit {
			break
		}
		out = append(out, h.it)
	}
	return out
}

// wordPrefix — какое-то слово s начинается с w.
func wordPrefix(s, w string) bool {
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if strings.HasPrefix(f, w) {
			return true
		}
	}
	return false
}

var (
	layoutEN = []rune("`qwertyuiop[]asdfghjkl;'zxcvbnm,.")
	layoutRU = []rune("ёйцукенгшщзхъфывапролджэячсмитьбю")
)

// swapLayout — текст, набранный не в той раскладке (ЙЦУКЕН ↔ QWERTY).
func swapLayout(s string) string {
	en, ru := map[rune]rune{}, map[rune]rune{}
	for i := range layoutEN {
		en[layoutEN[i]], ru[layoutRU[i]] = layoutRU[i], layoutEN[i]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if x, ok := en[r]; ok {
			b.WriteRune(x)
		} else if x, ok := ru[r]; ok {
			b.WriteRune(x)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------- отрисовка строки поиска ----------

// searchBox — строка поиска вместо вкладок; возвращает y под ней.
func (r *Renderer) searchBox(img *image.RGBA, L *Layout, v MenuView, x0, y, innerW float64, px func(float64) float64) float64 {
	const gw = 22.0
	box := rect{x0, y, innerW - px(gw+6), px(tabsH)}
	fillRound(img, box, px(10), color.NRGBA{255, 255, 255, 16})
	strokeRound(img, box, px(10), px(1.5), withAlpha(colAccent, 220))
	// лупа
	cx, cy, rad := box.x+px(15), box.y+box.h/2-px(1), px(5.5)
	strokeRound(img, rect{cx - rad, cy - rad, 2 * rad, 2 * rad}, rad, px(1.8), colDim)
	thickLine(img, cx+rad*0.7, cy+rad*0.7, cx+rad*1.55, cy+rad*1.55, px(2), colDim)

	tx := box.x + px(30)
	maxW := box.x + box.w - px(10) - tx
	base := box.y + (box.h+capHeight(r.F.face(r.F.Bold, px(14))))/2
	if v.Query == "" {
		f := r.F.face(r.F.Regular, px(13))
		text(img, f, ellipsize(f, v.Hint, maxW-px(4)), tx+px(4), box.y+(box.h+capHeight(f))/2, colFaint)
		caret(img, tx, box, px)
	} else {
		f := r.F.face(r.F.Bold, px(14))
		s := tailFit(f, v.Query, maxW-px(6))
		text(img, f, s, tx, base, colText)
		caret(img, tx+measure(f, s)+px(2), box, px)
	}
	g := rect{x0 + innerW - px(gw), y, px(gw), px(tabsH)}
	gripDots(img, g, colFaint, px)
	L.Grip = g.image()
	return y + px(tabsH+8)
}

// tailFit — конец строки, который помещается в ширину (начало — «…»).
func tailFit(f fontFace, s string, w float64) string {
	if measure(f, s) <= w {
		return s
	}
	r := []rune(s)
	for i := 1; i < len(r); i++ {
		if t := "…" + string(r[i:]); measure(f, t) <= w {
			return t
		}
	}
	return "…"
}

func caret(img *image.RGBA, x float64, box rect, px func(float64) float64) {
	fillRound(img, rect{x, box.y + px(9), px(2), box.h - px(18)}, px(1), colAccent)
}

// thickLine — отрезок толщиной w со сглаживанием.
func thickLine(dst *image.RGBA, x1, y1, x2, y2, w float64, c color.Color) {
	b := dst.Bounds()
	dx, dy := x2-x1, y2-y1
	l := max(1e-6, math.Hypot(dx, dy))
	nx, ny := -dy/l*w/2, dx/l*w/2
	ras := vector.NewRasterizer(b.Dx(), b.Dy())
	ras.MoveTo(float32(x1+nx), float32(y1+ny))
	ras.LineTo(float32(x2+nx), float32(y2+ny))
	ras.LineTo(float32(x2-nx), float32(y2-ny))
	ras.LineTo(float32(x1-nx), float32(y1-ny))
	ras.ClosePath()
	m := image.NewAlpha(b)
	ras.Draw(m, b, image.Opaque, image.Point{})
	draw.DrawMask(dst, b, image.NewUniform(c), image.Point{}, m, b.Min, draw.Over)
}
