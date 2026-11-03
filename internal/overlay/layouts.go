package overlay

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// Полоска и сетка: те же пункты, что в списке, но плитками.

const (
	stPad    = 8.0   // поля полоски
	stTileW  = 92.0  // плитка полоски
	stTileH  = 64.0  //
	stGap    = 6.0   // между плитками
	stGripW  = 16.0  // ручка ⠿
	stTabsW  = 74.0  // вкладки столбиком
	stPageW  = 30.0  // стрелки страниц
	stSec    = 8.0   // между частями полоски
	stEmptyW = 330.0 // «пусто»

	grPad   = 10.0
	grTileW = 108.0
	grTileH = 88.0
	grGap   = 8.0
	grCols  = 5
)

// tileStyle — размеры содержимого плитки (в логических пикселях от её верхнего левого угла).
type tileStyle struct {
	key, keyFont  float64 // квадратик с цифрой
	emoji, emojiY float64 // значок
	label, labelY float64 // подпись: размер шрифта и базовая линия
	hint, hintY   float64 // клавиша под подписью (0 — не показывать)
	radius        float64
}

var (
	stripTile = tileStyle{key: 17, keyFont: 11, emoji: 28, emojiY: 8, label: 12, labelY: 54, radius: 10}
	gridTile  = tileStyle{key: 20, keyFont: 12, emoji: 38, emojiY: 12, label: 13, labelY: 67, hint: 10.5, hintY: 81, radius: 12}
)

// fitScale — уменьшить масштаб, если картинка шире maxW.
func fitScale(s, logicalW float64, maxW int) float64 {
	if maxW > 0 {
		if full := logicalW * s; full > float64(maxW) {
			return max(s*float64(maxW)/full, s*0.55)
		}
	}
	return s
}

// strip — полоска: ⠿ · вкладки столбиком · до 10 плиток в ряд · стрелки страниц.
func (r *Renderer) strip(v MenuView) (*image.RGBA, Layout) {
	n := len(v.Items)
	slots := min(max(v.Slots, n), MenuPage)
	tabs := len(v.Tabs) > 1
	pages := v.Pages > 1
	tilesW := stEmptyW
	if slots > 0 {
		tilesW = float64(slots)*stTileW + float64(slots-1)*stGap
	}
	w := stPad + stGripW + stSec + tilesW + stPad
	if tabs {
		w += stTabsW + stSec
	}
	if pages {
		w += stSec + stPageW
	}
	h := stTileH + 2*stPad
	s := v.Scale
	if s <= 0 {
		s = 1
	}
	s = fitScale(s, w+2*margin, v.MaxW)
	px := func(x float64) float64 { return x * s }
	W, H := int((w+2*margin)*s+0.5), int((h+2*margin)*s+0.5)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	L := Layout{W: W, H: H}
	panel := rect{px(margin), px(margin), px(w), px(h)}
	L.Panel = panel.image()
	panelBG(img, panel, px(14), v.Opacity, px)
	bold := func(sz float64) fontFace { return r.F.face(r.F.Bold, px(sz)) }

	x, y := panel.x+px(stPad), panel.y+px(stPad)
	th := px(stTileH)
	g := rect{x, y, px(stGripW), th}
	gripDots(img, g, colFaint, px)
	L.Grip = g.image()
	x += px(stGripW + stSec)

	if tabs {
		track := rect{x, y, px(stTabsW), th}
		fillRound(img, track, px(10), colTabTrack)
		k := float64(len(v.Tabs))
		ph := (th - px(4) - px(2)*(k-1)) / k
		for i, t := range v.Tabs {
			tr := rect{x + px(2), y + px(2) + float64(i)*(ph+px(2)), px(stTabsW - 4), ph}
			L.Tabs = append(L.Tabs, tr.image())
			c := colDim
			if i == v.Tab {
				fillRound(img, tr, px(8), colAccent)
				c = colText
			}
			textCentered(img, bold(12.5), t, tr, c)
		}
		x += px(stTabsW + stSec)
	}

	if slots == 0 {
		msg := v.Empty
		if msg == "" {
			msg = "Пусто — добавьте звуки в VoiceBox"
		}
		f := r.F.face(r.F.Regular, px(13))
		textCentered(img, f, ellipsize(f, msg, px(stEmptyW-16)), rect{x, y, px(stEmptyW), th}, colDim)
	}
	for i := 0; i < slots; i++ {
		t := rect{x + float64(i)*px(stTileW+stGap), y, px(stTileW), th}
		if i < n {
			L.Rows = append(L.Rows, t.image())
			r.tile(img, t, v.Items[i], i == v.Hover, stripTile, px)
		} else {
			strokeRound(img, t, px(10), px(1), colSlot)
		}
	}
	x += px(tilesW)

	if pages {
		x += px(stSec)
		bw, bh := px(stPageW), px(20)
		prev := rect{x, y, bw, bh}
		next := rect{x, y + th - bh, bw, bh}
		for _, b := range []rect{prev, next} {
			fillRound(img, b, px(7), colTabTrack)
		}
		arrow(img, prev, true, colDim, px)
		arrow(img, next, false, colDim, px)
		L.Prev, L.Next = prev.image(), next.image()
		textCentered(img, bold(11.5), itoa(v.Page+1)+"/"+itoa(v.Pages), rect{x, y + bh, bw, th - 2*bh}, colDim)
	}
	return img, L
}

// grid — сетка 5×2 крупных плиток (как у пульта стримера).
func (r *Renderer) grid(v MenuView) (*image.RGBA, Layout) {
	n := len(v.Items)
	slots := min(max(v.Slots, n), MenuPage)
	rows := max((slots+grCols-1)/grCols, 1)
	innerW := grCols*grTileW + (grCols-1)*grGap
	head := 26.0
	if len(v.Tabs) > 1 {
		head = tabsH + 8
	}
	h := grPad + head + float64(rows)*grTileH + float64(rows-1)*grGap + 8 + footerH + grPad
	w := innerW + 2*grPad
	s := v.Scale
	if s <= 0 {
		s = 1
	}
	s = fitScale(s, w+2*margin, v.MaxW)
	px := func(x float64) float64 { return x * s }
	W, H := int((w+2*margin)*s+0.5), int((h+2*margin)*s+0.5)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	L := Layout{W: W, H: H}
	panel := rect{px(margin), px(margin), px(w), px(h)}
	L.Panel = panel.image()
	panelBG(img, panel, px(16), v.Opacity, px)

	x0, y := panel.x+px(grPad), panel.y+px(grPad)
	y = r.header(img, &L, v, x0, y, px(innerW), px)
	if slots == 0 {
		msg := v.Empty
		if msg == "" {
			msg = "Пусто — добавьте звуки в VoiceBox"
		}
		textCentered(img, r.F.face(r.F.Regular, px(13)), msg, rect{x0, y, px(innerW), px(grTileH)}, colDim)
	} else {
		for i := 0; i < rows*grCols; i++ {
			col, row := i%grCols, i/grCols
			t := rect{x0 + float64(col)*px(grTileW+grGap), y + float64(row)*px(grTileH+grGap), px(grTileW), px(grTileH)}
			if i < n {
				L.Rows = append(L.Rows, t.image())
				r.tile(img, t, v.Items[i], i == v.Hover, gridTile, px)
			} else {
				strokeRound(img, t, px(12), px(1), colSlot)
			}
		}
	}
	y += float64(rows)*px(grTileH) + float64(rows-1)*px(grGap) + px(8)
	r.footer(img, v, x0, y, px(innerW), px)
	return img, L
}

// tile — плитка: цифра в углу, значок, подпись (и клавиша в сетке).
func (r *Renderer) tile(img *image.RGBA, t rect, it Item, hover bool, st tileStyle, px func(float64) float64) {
	rad := px(st.radius)
	fillRound(img, t, rad, colTile)
	if it.Active {
		fillRound(img, t, rad, withAlpha(colAccent, 46))
	}
	if hover {
		fillRound(img, t, rad, colHover)
	}
	if it.Active {
		strokeRound(img, t, rad, px(1), withAlpha(colAccent, 130))
	} else if hover {
		strokeRound(img, t, rad, px(1), color.NRGBA{255, 255, 255, 40})
	}
	kb := rect{t.x + px(6), t.y + px(6), px(st.key), px(st.key)}
	fillRound(img, kb, px(5), colKeyBG)
	textCentered(img, r.F.face(r.F.Bold, px(st.keyFont)), it.Key, kb, colText)
	if it.Active {
		bars(img, rect{t.x + t.w - px(6+12), t.y + px(8), px(12), px(12)}, colAccent)
	}
	eb := rect{t.x + (t.w-px(st.emoji))/2, t.y + px(st.emojiY), px(st.emoji), px(st.emoji)}
	r.badge(img, eb, it.Emoji, it.Label, rgba(it.Color, colAccent), px)
	lf := r.F.face(r.F.Regular, px(st.label))
	c := colText
	if it.Pending {
		c = colDim
	}
	label := ellipsize(lf, it.Label, t.w-px(10))
	text(img, lf, label, t.x+(t.w-measure(lf, label))/2, t.y+px(st.labelY), c)
	if st.hint > 0 {
		hint := it.Hint
		if it.Pending {
			hint = "готовится…"
		}
		if hint != "" {
			hf := r.F.face(r.F.Regular, px(st.hint))
			hint = ellipsize(hf, hint, t.w-px(10))
			text(img, hf, hint, t.x+(t.w-measure(hf, hint))/2, t.y+px(st.hintY), colFaint)
		}
	}
}

// gripDots — ручка ⠿ (2×3 точки) по центру b.
func gripDots(img *image.RGBA, b rect, c color.NRGBA, px func(float64) float64) {
	d, sp := px(3), px(5)
	x0 := b.x + (b.w-(sp+d))/2
	y0 := b.y + (b.h-(2*sp+d))/2
	for i := 0; i < 2; i++ {
		for j := 0; j < 3; j++ {
			fillRound(img, rect{x0 + float64(i)*sp, y0 + float64(j)*sp, d, d}, d/2, c)
		}
	}
}

// arrow — треугольник ▲/▼ по центру b.
func arrow(img *image.RGBA, b rect, up bool, c color.NRGBA, px func(float64) float64) {
	cx, cy := b.x+b.w/2, b.y+b.h/2
	w, h := px(4.5), px(2.75)
	if up {
		fillPoly(img, [][2]float64{{cx - w, cy + h}, {cx + w, cy + h}, {cx, cy - h}}, c)
	} else {
		fillPoly(img, [][2]float64{{cx - w, cy - h}, {cx + w, cy - h}, {cx, cy + h}}, c)
	}
}

// fillPoly — сглаженный многоугольник.
func fillPoly(dst *image.RGBA, pts [][2]float64, c color.Color) {
	if len(pts) < 3 {
		return
	}
	x0, y0, x1, y1 := pts[0][0], pts[0][1], pts[0][0], pts[0][1]
	for _, p := range pts[1:] {
		x0, y0, x1, y1 = min(x0, p[0]), min(y0, p[1]), max(x1, p[0]), max(y1, p[1])
	}
	b := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1)))
	if b.Intersect(dst.Bounds()).Empty() {
		return
	}
	ras := vector.NewRasterizer(b.Dx(), b.Dy())
	ox, oy := float64(b.Min.X), float64(b.Min.Y)
	ras.MoveTo(float32(pts[0][0]-ox), float32(pts[0][1]-oy))
	for _, p := range pts[1:] {
		ras.LineTo(float32(p[0]-ox), float32(p[1]-oy))
	}
	ras.ClosePath()
	m := image.NewAlpha(image.Rect(0, 0, b.Dx(), b.Dy()))
	ras.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	draw.DrawMask(dst, b, image.NewUniform(c), image.Point{}, m, image.Point{}, draw.Over)
}
