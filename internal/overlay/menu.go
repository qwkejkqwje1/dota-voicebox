package overlay

import (
	"image"
	"image/color"
	"strings"
)

// Item — строка меню.
type Item struct {
	ID      string
	Key     string // "1".."0"
	Emoji   string
	Label   string
	Color   string // #rrggbb
	Hint    string // глобальная клавиша (подсказка справа)
	Active  bool   // звук играет / эффект включён
	Pending bool   // ещё готовится (синтез речи)
	Kind    string // для поиска: sound | effect | say ("" — как вкладка)
	Find    string // для поиска: дополнительные слова (группа, текст фразы)
}

// Виды меню.
const (
	LayoutList  = "list"  // список строк (классика)
	LayoutStrip = "strip" // полоска плиток в один ряд — сверху или снизу экрана
	LayoutGrid  = "grid"  // сетка 5×2 крупных плиток
)

// MenuView — что показать.
type MenuView struct {
	Layout  string   // list | strip | grid
	Tabs    []string // ["Звуки", "Голос"]; одна вкладка — без переключателя
	Tab     int
	Items   []Item
	Slots   int // мест в полоске/сетке: размер не прыгает между вкладками и страницами (0 — по числу пунктов)
	Page    int
	Pages   int
	Hover   int    // пункт под мышью или выбранный стрелками (-1 — нет)
	Footer  string // подсказки внизу
	Empty   string // текст, если пунктов нет
	Scale   float64
	Opacity float64 // непрозрачность фона 0.3..1 (0 — обычная)
	MaxW    int     // предельная ширина картинки, px (полоска ужимается под экран); 0 — без ограничения
	Typing  bool    // режим набора: вместо вкладок — строка поиска
	Query   string  // что набрано
	Hint    string  // подсказка в пустой строке поиска
}

// Layout — где что нарисовано (для мыши).
type Layout struct {
	W, H       int
	Panel      image.Rectangle
	Rows       []image.Rectangle // пункты (строки или плитки)
	Tabs       []image.Rectangle
	Grip       image.Rectangle // ⠿ — потянуть, чтобы передвинуть меню
	Prev, Next image.Rectangle // стрелки страниц (полоска)
}

// HitTest — что под точкой: ("row", i), ("tab", i), ("prev"/"next", -1),
// ("grip", -1) — ручка, ("panel", -1) — фон меню (за него тоже можно тащить), ("", -1) — мимо.
func (l Layout) HitTest(x, y int) (string, int) {
	p := image.Pt(x, y)
	for i, r := range l.Rows {
		if p.In(r) {
			return "row", i
		}
	}
	for i, r := range l.Tabs {
		if p.In(r) {
			return "tab", i
		}
	}
	switch {
	case p.In(l.Prev):
		return "prev", -1
	case p.In(l.Next):
		return "next", -1
	case p.In(l.Grip):
		return "grip", -1
	case p.In(l.Panel):
		return "panel", -1
	}
	return "", -1
}

// Renderer рисует меню и уведомления.
type Renderer struct {
	F     *Fonts
	Emoji EmojiSource // цветные эмодзи (может быть nil)
}

var (
	colBG       = color.NRGBA{17, 17, 23, 242}
	colTile     = color.NRGBA{255, 255, 255, 11}
	colSlot     = color.NRGBA{255, 255, 255, 12}
	colBorder   = color.NRGBA{255, 255, 255, 24}
	colText     = color.NRGBA{244, 244, 245, 255}
	colDim      = color.NRGBA{161, 161, 170, 255}
	colFaint    = color.NRGBA{113, 113, 122, 255}
	colAccent   = color.NRGBA{139, 92, 246, 255}
	colHover    = color.NRGBA{255, 255, 255, 18}
	colKeyBG    = color.NRGBA{255, 255, 255, 22}
	colTabTrack = color.NRGBA{255, 255, 255, 14}
)

// MenuSize — размер меню для числа строк (чтобы заранее поставить окно).
func MenuSize(rows int, tabs bool, scale float64) (int, int) {
	s := scale
	if s <= 0 {
		s = 1
	}
	h := menuHeight(rows, tabs)
	return int((menuW + 2*margin) * s), int((h + 2*margin) * s)
}

const (
	margin  = 18.0
	menuW   = 344.0
	pad     = 10.0
	tabsH   = 34.0
	rowH    = 40.0
	rowGap  = 2.0
	footerH = 28.0
)

func menuHeight(rows int, tabs bool) float64 {
	if rows < 1 {
		rows = 1
	}
	h := pad + float64(rows)*(rowH+rowGap) - rowGap + 6 + footerH + pad
	if tabs {
		h += tabsH + 8
	} else {
		h += 26 // заголовок
	}
	return h
}

// Menu рисует меню. Картинка — RGBA с premultiplied alpha (как нужно UpdateLayeredWindow).
func (r *Renderer) Menu(v MenuView) (*image.RGBA, Layout) {
	switch v.Layout {
	case LayoutStrip:
		return r.strip(v)
	case LayoutGrid:
		return r.grid(v)
	}
	return r.list(v)
}

// panelBG — фон панели с тенью; прозрачнее фон — слабее тень (иначе она просвечивает).
func panelBG(img *image.RGBA, panel rect, rad float64, opacity float64, px func(float64) float64) {
	if opacity <= 0 || opacity > 1 {
		opacity = float64(colBG.A) / 255
	}
	opacity = max(opacity, 0.3)
	shadow(img, panel, rad, px(16), px(6), uint8(150*opacity*opacity))
	fillRound(img, panel, rad, withAlpha(colBG, uint8(opacity*255+0.5)))
	strokeRound(img, panel, rad, px(1), colBorder)
}

// list — список строк.
func (r *Renderer) list(v MenuView) (*image.RGBA, Layout) {
	s := v.Scale
	if s <= 0 {
		s = 1
	}
	rows := len(v.Items)
	tabs := len(v.Tabs) > 1 || v.Typing
	h := menuHeight(rows, tabs)
	W, H := int((menuW+2*margin)*s+0.5), int((h+2*margin)*s+0.5)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	L := Layout{W: W, H: H}

	px := func(x float64) float64 { return x * s }
	panel := rect{px(margin), px(margin), px(menuW), px(h)}
	L.Panel = panel.image()
	panelBG(img, panel, px(16), v.Opacity, px)

	reg := func(sz float64) fontFace { return r.F.face(r.F.Regular, px(sz)) }
	bold := func(sz float64) fontFace { return r.F.face(r.F.Bold, px(sz)) }

	x0, y := panel.x+px(pad), panel.y+px(pad)
	innerW := panel.w - px(2*pad)
	if v.Typing {
		y = r.searchBox(img, &L, v, x0, y, innerW, px)
	} else {
		y = r.header(img, &L, v, x0, y, innerW, px)
	}

	if rows == 0 {
		msg := v.Empty
		if msg == "" {
			msg = "Пусто — добавьте звуки в VoiceBox"
		}
		textCentered(img, reg(13), msg, rect{x0, y, innerW, px(rowH)}, colDim)
		y += px(rowH + rowGap)
	}
	for i, it := range v.Items {
		row := rect{x0, y, innerW, px(rowH)}
		L.Rows = append(L.Rows, row.image())
		switch {
		case it.Active:
			fillRound(img, row, px(10), withAlpha(colAccent, 46))
			if i == v.Hover {
				fillRound(img, row, px(10), colHover)
			}
		case i == v.Hover:
			fillRound(img, row, px(10), colHover)
		}
		// клавиша (в поиске её нет — значок левее)
		ex := 38.0
		if it.Key != "" {
			kb := rect{row.x + px(8), row.y + (row.h-px(22))/2, px(22), px(22)}
			fillRound(img, kb, px(6), colKeyBG)
			textCentered(img, bold(12), it.Key, kb, colText)
		} else if v.Typing {
			ex = 8
		}
		// значок
		eb := rect{row.x + px(ex), row.y + (row.h-px(28))/2, px(28), px(28)}
		r.badge(img, eb, it.Emoji, it.Label, rgba(it.Color, colAccent), px)
		// подпись и подсказка справа
		right := row.x + row.w - px(10)
		if it.Active {
			bars(img, rect{right - px(14), row.y + px(13), px(14), px(14)}, colAccent)
			right -= px(22)
		} else if it.Pending {
			text(img, reg(11.5), "готовится…", right-measure(reg(11.5), "готовится…"), row.y+(row.h+capHeight(reg(11.5)))/2, colFaint)
			right -= measure(reg(11.5), "готовится…") + px(8)
		}
		if it.Hint != "" && !it.Pending {
			hf := reg(11.5)
			hw := measure(hf, it.Hint)
			text(img, hf, it.Hint, right-hw, row.y+(row.h+capHeight(hf))/2, colFaint)
			right -= hw + px(10)
		}
		lf := reg(14.5)
		lx := row.x + px(ex+38)
		c := colText
		if it.Pending {
			c = colDim
		}
		text(img, lf, ellipsize(lf, it.Label, right-lx), lx, row.y+(row.h+capHeight(lf))/2, c)
		y += px(rowH + rowGap)
	}
	y += px(6 - rowGap)
	r.footer(img, v, x0, y, innerW, px)
	return img, L
}

// header — вкладки «Звуки | Голос» (или заголовок) и ручка ⠿ справа; возвращает y под ними.
func (r *Renderer) header(img *image.RGBA, L *Layout, v MenuView, x0, y, innerW float64, px func(float64) float64) float64 {
	bold := func(sz float64) fontFace { return r.F.face(r.F.Bold, px(sz)) }
	const gw = 22.0
	if len(v.Tabs) > 1 {
		track := rect{x0, y, innerW - px(gw+6), px(tabsH)}
		fillRound(img, track, px(10), colTabTrack)
		tw := (track.w - px(4)) / float64(len(v.Tabs))
		for i, t := range v.Tabs {
			tr := rect{x0 + px(2) + float64(i)*tw, y + px(2), tw, px(tabsH - 4)}
			L.Tabs = append(L.Tabs, tr.image())
			c := colDim
			if i == v.Tab {
				fillRound(img, tr, px(8), colAccent)
				c = colText
			}
			label := t
			if i == v.Tab && v.Pages > 1 {
				label += "  " + itoa(v.Page+1) + "/" + itoa(v.Pages)
			}
			textCentered(img, bold(13), label, tr, c)
		}
		g := rect{x0 + innerW - px(gw), y, px(gw), px(tabsH)}
		gripDots(img, g, colFaint, px)
		L.Grip = g.image()
		return y + px(tabsH+8)
	}
	title := "Быстрое меню"
	if len(v.Tabs) == 1 {
		title = v.Tabs[0]
	}
	if v.Pages > 1 {
		title += "  " + itoa(v.Page+1) + "/" + itoa(v.Pages)
	}
	text(img, bold(12.5), strings.ToUpper(title), x0+px(4), y+px(16), colFaint)
	g := rect{x0 + innerW - px(gw), y, px(gw), px(22)}
	gripDots(img, g, colFaint, px)
	L.Grip = g.image()
	return y + px(26)
}

// footer — подсказки по клавишам под чертой.
func (r *Renderer) footer(img *image.RGBA, v MenuView, x0, y, innerW float64, px func(float64) float64) {
	ff := r.F.face(r.F.Regular, px(11.5))
	foot := v.Footer
	if foot == "" {
		foot = "1–0 выбор  ·  Esc закрыть"
	}
	fr := rect{x0, y, innerW, px(footerH)}
	sep := rect{x0 + px(4), y, innerW - px(8), px(1)}
	fillRound(img, sep, 0, color.NRGBA{255, 255, 255, 14})
	textCentered(img, ff, ellipsize(ff, foot, innerW), fr, colFaint)
}

// badge — цветной квадратик со значком: цветной эмодзи из интерфейса, контур из шрифта или буква.
func (r *Renderer) badge(img *image.RGBA, b rect, emoji, label string, c color.NRGBA, px func(float64) float64) {
	if r.Emoji != nil && emoji != "" {
		if e := r.Emoji.Emoji(emoji); e != nil {
			fillRound(img, b, px(8), withAlpha(c, 70))
			drawImageFit(img, e, rect{b.x + px(4), b.y + px(4), b.w - px(8), b.h - px(8)})
			return
		}
	}
	fillRound(img, b, px(8), c)
	if ru := firstRune(emoji); ru != 0 {
		if ef := r.F.face(r.F.emoji(), px(16)); ef != nil {
			if gb, _, ok := ef.GlyphBounds(ru); ok && gb.Max.X > gb.Min.X && gb.Max.Y > gb.Min.Y {
				w := float64(gb.Max.X-gb.Min.X) / 64
				hgt := float64(gb.Max.Y-gb.Min.Y) / 64
				x := b.x + (b.w-w)/2 - float64(gb.Min.X)/64
				y := b.y + (b.h-hgt)/2 - float64(gb.Min.Y)/64
				text(img, ef, string(ru), x, y, colText)
				return
			}
		}
	}
	letter := "•"
	for _, ch := range label {
		letter = strings.ToUpper(string(ch))
		break
	}
	textCentered(img, r.F.face(r.F.Bold, px(14)), letter, b, colText)
}

// bars — значок «играет» (три столбика).
func bars(img *image.RGBA, b rect, c color.NRGBA) {
	hs := []float64{0.55, 1, 0.75}
	w := b.w / 4.2
	for i, k := range hs {
		h := b.h * k
		fillRound(img, rect{b.x + float64(i)*(w*1.6), b.y + b.h - h, w, h}, w/2, c)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// ToastView — всплывающее уведомление.
type ToastView struct {
	Emoji string
	Text  string
	Sub   string
	Color string
	Scale float64
}

// Toast рисует уведомление-«пилюлю».
func (r *Renderer) Toast(t ToastView) *image.RGBA {
	s := t.Scale
	if s <= 0 {
		s = 1
	}
	px := func(x float64) float64 { return x * s }
	tf := r.F.face(r.F.Bold, px(14.5))
	sf := r.F.face(r.F.Regular, px(11.5))
	tw := measure(tf, t.Text)
	if t.Sub != "" {
		tw = max(tw, measure(sf, t.Sub))
	}
	w := min(max(px(16+34+12)+tw+px(22), px(200)), px(440))
	h := px(56)
	m := px(margin)
	img := image.NewRGBA(image.Rect(0, 0, int(w+2*m+0.5), int(h+2*m+0.5)))
	p := rect{m, m, w, h}
	shadow(img, p, px(16), px(16), px(5), 150)
	fillRound(img, p, px(16), colBG)
	strokeRound(img, p, px(16), px(1), colBorder)
	b := rect{p.x + px(12), p.y + (h-px(34))/2, px(34), px(34)}
	r.badge(img, b, t.Emoji, t.Text, rgba(t.Color, colAccent), px)
	tx := b.x + b.w + px(12)
	maxW := p.x + p.w - px(14) - tx
	if t.Sub == "" {
		text(img, tf, ellipsize(tf, t.Text, maxW), tx, p.y+(h+capHeight(tf))/2, colText)
	} else {
		text(img, tf, ellipsize(tf, t.Text, maxW), tx, p.y+px(24), colText)
		text(img, sf, ellipsize(sf, t.Sub, maxW), tx, p.y+px(41), colDim)
	}
	return img
}
