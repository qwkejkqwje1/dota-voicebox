package overlay

import "image"

// Positions — где может появляться меню. custom — своё место (меню перетащили мышью).
var Positions = []string{"top-left", "top", "top-right", "left", "center", "right",
	"bottom-left", "bottom", "bottom-right", "cursor", "custom"}

// Placement — где показать меню.
type Placement struct {
	Position string
	X, Y     float64     // custom: доля свободного места (0 — у левого/верхнего края, 1 — у правого/нижнего)
	Cursor   image.Point // курсор мыши (для cursor)
	Scale    float64     // масштаб экрана (отступ у курсора)
}

// Place — левый верхний угол картинки меню w×h в области area. panel — сама панель внутри картинки
// (вокруг неё прозрачные поля под тень): панель всегда целиком на экране, тень может уйти за край.
// Отступы готовых мест обходят интерфейс игр: сверху 8% (счёт, часы), снизу 20% (панель героя).
func Place(area image.Rectangle, w, h int, panel image.Rectangle, p Placement) image.Point {
	aw, ah := area.Dx(), area.Dy()
	cx, cy := area.Min.X+(aw-w)/2, area.Min.Y+(ah-h)/2
	top := area.Min.Y + ah*8/100
	bottom := area.Max.Y - h - ah*20/100
	left := area.Min.X + aw*2/100
	right := area.Max.X - w - aw*2/100
	var o image.Point
	switch p.Position {
	case "top":
		o = image.Pt(cx, top)
	case "bottom":
		o = image.Pt(cx, bottom)
	case "left":
		o = image.Pt(left, cy)
	case "right":
		o = image.Pt(right, cy)
	case "top-left":
		o = image.Pt(left, top)
	case "top-right":
		o = image.Pt(right, top)
	case "bottom-left":
		o = image.Pt(left, bottom)
	case "bottom-right":
		o = image.Pt(right, bottom)
	case "cursor":
		s := p.Scale
		if s <= 0 {
			s = 1
		}
		o = image.Pt(p.Cursor.X-w/2, p.Cursor.Y-int(margin*s))
	case "custom":
		lo, hi := freeRange(area, panel)
		o = image.Pt(lo.X+int(float64(hi.X-lo.X)*clamp01(p.X)+0.5), lo.Y+int(float64(hi.Y-lo.Y)*clamp01(p.Y)+0.5))
	default:
		o = image.Pt(cx, cy)
	}
	return Clamp(area, o, panel)
}

// freeRange — где может стоять левый верхний угол картинки, чтобы панель была на экране.
func freeRange(area, panel image.Rectangle) (lo, hi image.Point) {
	lo = area.Min.Sub(panel.Min)
	hi = area.Max.Sub(panel.Max)
	hi.X, hi.Y = max(hi.X, lo.X), max(hi.Y, lo.Y)
	return lo, hi
}

// Clamp — не дать панели уйти за край области.
func Clamp(area image.Rectangle, o image.Point, panel image.Rectangle) image.Point {
	lo, hi := freeRange(area, panel)
	return image.Pt(min(max(o.X, lo.X), hi.X), min(max(o.Y, lo.Y), hi.Y))
}

// CustomXY — место меню как доли свободного места (обратное к Place для custom).
func CustomXY(area image.Rectangle, o image.Point, panel image.Rectangle) (x, y float64) {
	lo, hi := freeRange(area, panel)
	frac := func(v, a, b int) float64 {
		if b <= a {
			return 0.5
		}
		return clamp01(float64(v-a) / float64(b-a))
	}
	return round3(frac(o.X, lo.X, hi.X)), round3(frac(o.Y, lo.Y, hi.Y))
}

// Align — какой край держать, когда меню меняет размер (другая вкладка, меньше строк):
// 0 — левый/верхний, 0.5 — середину, 1 — правый/нижний. По вертикали середина не держится:
// верх стоит на месте, чтобы вкладки не уезжали из-под мыши.
func Align(p Placement) (ax, ay float64) {
	ax = 0.5
	switch p.Position {
	case "left", "top-left", "bottom-left":
		ax = 0
	case "right", "top-right", "bottom-right":
		ax = 1
	case "custom":
		switch {
		case p.X < 1.0/3:
			ax = 0
		case p.X > 2.0/3:
			ax = 1
		}
	}
	switch p.Position {
	case "bottom", "bottom-left", "bottom-right":
		ay = 1
	case "custom":
		if p.Y > 2.0/3 {
			ay = 1
		}
	}
	return ax, ay
}

// Reanchor — новый угол меню после смены размера old → cur с теми же краями.
func Reanchor(o, old, cur image.Point, ax, ay float64) image.Point {
	return image.Pt(o.X+int(float64(old.X-cur.X)*ax), o.Y+int(float64(old.Y-cur.Y)*ay))
}

func clamp01(v float64) float64 {
	if v < 0 || v != v {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func round3(v float64) float64 { return float64(int(v*1000+0.5)) / 1000 }
