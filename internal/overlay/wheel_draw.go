package overlay

import (
	"image"
	"image/color"
	"math"
	"strings"
)

// Рисование колеса: 8 секторов-«долек» с постоянными зазорами, выбранный выдвигается и
// подсвечивается, в середине — название выбранного и подсказка, стрелка показывает направление мыши.

var (
	colWheelSector = color.NRGBA{24, 24, 32, 255}
	colWheelBack   = color.NRGBA{8, 8, 12, 255}
)

// Wheel рисует колесо целиком (со стрелкой).
func (r *Renderer) Wheel(v WheelView) *image.RGBA {
	img := r.WheelBase(v)
	WheelPointer(img, v)
	return img
}

func wheelScale(v WheelView) (float64, func(float64) float64) {
	s := v.Scale
	if s <= 0 {
		s = 1
	}
	return s, func(x float64) float64 { return x * s }
}

func wheelOpacity(v WheelView) float64 {
	if v.Opacity <= 0 {
		return 0.92
	}
	return math.Min(v.Opacity, 1)
}

func circle(cx, cy, r float64) rect { return rect{cx - r, cy - r, 2 * r, 2 * r} }

// WheelBase — колесо без стрелки (стрелка меняется при каждом движении мыши, остальное — редко).
func (r *Renderer) WheelBase(v WheelView) *image.RGBA {
	s, px := wheelScale(v)
	side := WheelSize(s)
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	c := float64(side) / 2
	op := wheelOpacity(v)
	R := px(whRout + 3)
	shadow(img, circle(c, c, R), R, px(20), px(5), uint8(80+60*op))
	fillRound(img, circle(c, c, px(whRout+4)), px(whRout+4), withAlpha(colWheelBack, uint8(120*op)))
	for i := 0; i < WheelSlots && i < len(v.Items); i++ {
		if i != v.Hover {
			r.wheelSector(img, v, i, c, px, op)
		}
	}
	if v.Hover >= 0 && v.Hover < len(v.Items) { // выбранный — последним, поверх соседей
		r.wheelSector(img, v, v.Hover, c, px, op)
	}
	r.wheelCenter(img, v, c, px, op)
	return img
}

// sectorPoly — «долька» от угла a0 до a1 (радианы от верха по часовой) между радиусами rin и rout;
// края сдвинуты на half внутрь параллельно — зазор между соседями одинаковой ширины по всей длине.
func sectorPoly(cx, cy, rin, rout, a0, a1, half float64) [][2]float64 {
	var pts [][2]float64
	arc := func(rad, from, to float64, n int) {
		for k := 0; k <= n; k++ {
			a := from + (to-from)*float64(k)/float64(n)
			pts = append(pts, [2]float64{cx + rad*math.Sin(a), cy - rad*math.Cos(a)})
		}
	}
	do := math.Asin(math.Min(half/rout, 1))
	di := math.Asin(math.Min(half/rin, 1))
	arc(rout, a0+do, a1-do, 32)
	arc(rin, a1-di, a0+di, 14)
	return pts
}

// angleDiff — разница углов в (-π, π].
func angleDiff(a, b float64) float64 {
	d := math.Mod(a-b, 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	} else if d <= -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

func (r *Renderer) wheelSector(img *image.RGBA, v WheelView, i int, c float64, px func(float64) float64, op float64) {
	it := v.Items[i]
	step := 2 * math.Pi / WheelSlots
	mid := float64(i) * step
	a0, a1 := mid-step/2, mid+step/2
	hov := i == v.Hover
	cx, cy := c, c
	if hov {
		cx, cy = c+px(whPop)*math.Sin(mid), c-px(whPop)*math.Cos(mid)
	}
	half := px(whGap / 2)
	poly := sectorPoly(cx, cy, px(whRin), px(whRout), a0, a1, half)
	col := rgba(it.Color, colAccent)
	switch {
	case it.Act == "":
		fillPoly(img, poly, withAlpha(colWheelSector, uint8(140*op)))
	case hov:
		fillPoly(img, poly, withAlpha(colAccent, 240))
		fillPoly(img, sectorPoly(cx, cy, px(whRout-3), px(whRout), a0, a1, half), color.NRGBA{255, 255, 255, 120})
	default:
		fillPoly(img, poly, withAlpha(colWheelSector, uint8(235*op)))
		fillPoly(img, poly, withAlpha(col, uint8(18+22*op)))
		if it.Active {
			fillPoly(img, sectorPoly(cx, cy, px(whRout-4), px(whRout), a0, a1, half), withAlpha(colAccent, 230))
		}
	}

	// номер сектора (клавиши 1–8) — у внешнего края, ближе к началу дольки
	da := 10 * math.Pi / 180
	nr := px(whRout - 14)
	nx, ny := cx+nr*math.Sin(a0+da), cy-nr*math.Cos(a0+da)
	nf := r.F.face(r.F.Bold, px(10.5))
	ncol := colFaint
	if hov {
		ncol = color.NRGBA{255, 255, 255, 200}
	}
	textCentered(img, nf, itoa(i+1), rect{nx - px(8), ny - px(8), px(16), px(16)}, ncol)
	if it.Act == "" {
		return
	}

	// значок и подпись: блок по центру дольки, подпись с той стороны, где ей просторнее
	rc := px((whRin+whRout)/2 + 2)
	bx, by := cx+rc*math.Sin(mid), cy-rc*math.Cos(mid)
	bs := px(38)
	face := r.F.Regular
	if hov {
		face = r.F.Bold
	}
	lf := r.F.face(face, px(12))
	ch := capHeight(lf)
	gap := px(5)
	blockH := bs + gap + ch
	pad := px(3)
	inside := func(x, y float64) bool {
		dx, dy := x-cx, y-cy
		rr := math.Hypot(dx, dy)
		if rr < px(whRin)+pad || rr > px(whRout)-pad {
			return false
		}
		lim := step/2 - math.Asin(math.Min((half+pad)/rr, 1))
		return math.Abs(angleDiff(direction(dx, dy), mid)) <= lim
	}
	room := func(base float64) float64 { // половина ширины, свободная для подписи на этой высоте
		w, maxW := 0.0, px(70)
		for w < maxW {
			x0, x1 := bx-w-1, bx+w+1
			if !inside(x0, base) || !inside(x1, base) || !inside(x0, base-ch) || !inside(x1, base-ch) {
				break
			}
			w++
		}
		return w
	}
	top := by - blockH/2
	belowBase := top + blockH
	aboveBase := top + ch
	wb, wa := room(belowBase), room(aboveBase)
	badgeY, base, w := top, belowBase, wb
	if wa > wb+px(4) {
		badgeY, base, w = top+ch+gap, aboveBase, wa
	}
	b := rect{bx - bs/2, badgeY, bs, bs}
	r.badge(img, b, it.Emoji, it.Label, col, px)
	if it.Active {
		bc := colAccent
		if hov {
			bc = colText
		}
		bars(img, rect{b.x + b.w + px(1), b.y, px(11), px(11)}, bc)
	}
	tc := colText
	if it.Pending && !hov {
		tc = colDim
	}
	label := ellipsize(lf, it.Label, 2*w)
	text(img, lf, label, bx-measure(lf, label)/2, base, tc)
}

// wrapLines — до n строк по словам; что не влезло — «…» в последней.
func wrapLines(face fontFace, s string, w float64, n int) []string {
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for i, wd := range words {
		try := wd
		if cur != "" {
			try = cur + " " + wd
		}
		if measure(face, try) <= w || cur == "" {
			cur = try
			continue
		}
		lines = append(lines, cur)
		cur = wd
		if len(lines) == n-1 {
			cur = strings.Join(words[i:], " ")
			break
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	for i := range lines {
		lines[i] = ellipsize(face, lines[i], w)
	}
	return lines
}

func (r *Renderer) wheelCenter(img *image.RGBA, v WheelView, c float64, px func(float64) float64, op float64) {
	R := px(whCenterR)
	fillRound(img, circle(c, c, R), R, withAlpha(colBG, uint8(255*math.Max(op, 0.85))))
	strokeRound(img, circle(c, c, R), R, px(1), colBorder)
	var lines []string
	var hint string
	main := colText
	tf := r.F.face(r.F.Bold, px(13.5))
	if v.Hover >= 0 && v.Hover < len(v.Items) {
		lines = wrapLines(tf, v.Items[v.Hover].Label, px(92), 2)
		hint = "отпустите"
		if v.Sticky {
			hint = "щёлкните"
		}
	} else {
		tf = r.F.face(r.F.Regular, px(12.5))
		main = colDim
		lines = []string{"Ведите мышь", "к звуку"}
		if v.Sticky {
			lines, hint = []string{"Щёлкните", "по звуку"}, "Esc — закрыть"
		}
	}
	hf := r.F.face(r.F.Regular, px(10.5))
	lh := px(17)
	total := float64(len(lines)) * lh
	if hint != "" {
		total += px(15)
	}
	y := c - total/2 + capHeight(tf) + (lh-capHeight(tf))/2 - px(1)
	for _, l := range lines {
		text(img, tf, l, c-measure(tf, l)/2, y, main)
		y += lh
	}
	if hint != "" {
		text(img, hf, hint, c-measure(hf, hint)/2, y-lh+px(15)+capHeight(hf)/2+px(2), colFaint)
	}
}

// WheelPointer — стрелка на краю середины: куда показывает мышь.
func WheelPointer(img *image.RGBA, v WheelView) {
	if !v.PointerOn {
		return
	}
	s, px := wheelScale(v)
	c := float64(WheelSize(s)) / 2
	a := v.Pointer
	sn, cs := math.Sin(a), math.Cos(a)
	tip, base, w := px(whCenterR+8.5), px(whCenterR-1), px(7.5)
	bx, by := c+base*sn, c-base*cs
	col := colAccent
	if v.Hover >= 0 {
		col = colText
	}
	fillPoly(img, [][2]float64{{c + tip*sn, c - tip*cs}, {bx + w*cs, by + w*sn}, {bx - w*cs, by - w*sn}}, col)
}
