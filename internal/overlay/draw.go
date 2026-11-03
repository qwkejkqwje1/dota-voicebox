package overlay

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"unicode/utf8"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// rect — прямоугольник в float-координатах.
type rect struct{ x, y, w, h float64 }

func (r rect) image() image.Rectangle {
	return image.Rect(int(math.Floor(r.x)), int(math.Floor(r.y)), int(math.Ceil(r.x+r.w)), int(math.Ceil(r.y+r.h)))
}

func (r rect) contains(x, y int) bool {
	fx, fy := float64(x), float64(y)
	return fx >= r.x && fx < r.x+r.w && fy >= r.y && fy < r.y+r.h
}

// roundMask — сглаженная маска скруглённого прямоугольника размером с dst.
func roundMask(b image.Rectangle, r rect, rad float64) *image.Alpha {
	ras := vector.NewRasterizer(b.Dx(), b.Dy())
	x0, y0 := float32(r.x-float64(b.Min.X)), float32(r.y-float64(b.Min.Y))
	x1, y1 := x0+float32(r.w), y0+float32(r.h)
	rd := float32(math.Min(rad, math.Min(r.w, r.h)/2))
	const k = 0.5523 // приближение дуги кубической кривой
	c := rd * (1 - k)
	ras.MoveTo(x0+rd, y0)
	ras.LineTo(x1-rd, y0)
	ras.CubeTo(x1-c, y0, x1, y0+c, x1, y0+rd)
	ras.LineTo(x1, y1-rd)
	ras.CubeTo(x1, y1-c, x1-c, y1, x1-rd, y1)
	ras.LineTo(x0+rd, y1)
	ras.CubeTo(x0+c, y1, x0, y1-c, x0, y1-rd)
	ras.LineTo(x0, y0+rd)
	ras.CubeTo(x0, y0+c, x0+c, y0, x0+rd, y0)
	ras.ClosePath()
	m := image.NewAlpha(image.Rect(0, 0, b.Dx(), b.Dy()))
	ras.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

func fillRound(dst *image.RGBA, r rect, rad float64, c color.Color) {
	b := r.image().Intersect(dst.Bounds())
	if b.Empty() {
		return
	}
	m := roundMask(b, r, rad)
	draw.DrawMask(dst, b, image.NewUniform(c), image.Point{}, m, image.Point{}, draw.Over)
}

// strokeRound — рамка толщиной w (разность двух масок).
func strokeRound(dst *image.RGBA, r rect, rad, w float64, c color.Color) {
	b := r.image().Intersect(dst.Bounds())
	if b.Empty() {
		return
	}
	outer := roundMask(b, r, rad)
	inner := roundMask(b, rect{r.x + w, r.y + w, r.w - 2*w, r.h - 2*w}, rad-w)
	for i := range outer.Pix {
		if v := int(outer.Pix[i]) - int(inner.Pix[i]); v > 0 {
			outer.Pix[i] = uint8(v)
		} else {
			outer.Pix[i] = 0
		}
	}
	draw.DrawMask(dst, b, image.NewUniform(c), image.Point{}, outer, image.Point{}, draw.Over)
}

// shadow — мягкая тень под скруглённым прямоугольником.
func shadow(dst *image.RGBA, r rect, rad, blur, dy float64, alpha uint8) {
	b := dst.Bounds()
	m := roundMask(b, rect{r.x, r.y + dy, r.w, r.h}, rad)
	rad2 := int(blur/2 + 0.5)
	for i := 0; i < 3; i++ {
		boxBlur(m, rad2)
	}
	draw.DrawMask(dst, b, image.NewUniform(color.NRGBA{0, 0, 0, alpha}), image.Point{}, m, image.Point{}, draw.Over)
}

func boxBlur(m *image.Alpha, r int) {
	if r < 1 {
		return
	}
	w, h := m.Rect.Dx(), m.Rect.Dy()
	tmp := make([]uint8, len(m.Pix))
	pass := func(src, dst []uint8, n, count, stride, step int) {
		for line := 0; line < count; line++ {
			base := line * stride
			sum := 0
			for i := -r; i <= r; i++ {
				j := min(max(i, 0), n-1)
				sum += int(src[base+j*step])
			}
			for i := 0; i < n; i++ {
				dst[base+i*step] = uint8(sum / (2*r + 1))
				a := min(i+r+1, n-1)
				b := max(i-r, 0)
				sum += int(src[base+a*step]) - int(src[base+b*step])
			}
		}
	}
	pass(m.Pix, tmp, w, h, m.Stride, 1)
	pass(tmp, m.Pix, h, w, 1, m.Stride)
}

// text рисует строку; (x, y) — левый край и базовая линия.
func text(dst *image.RGBA, face font.Face, s string, x, y float64, c color.Color) {
	if face == nil {
		return
	}
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: face, Dot: fixed.P(int(math.Round(x)), int(math.Round(y)))}
	d.DrawString(s)
}

func measure(face font.Face, s string) float64 {
	if face == nil {
		return 0
	}
	return float64(font.MeasureString(face, s)) / 64
}

// ellipsize обрезает строку с «…», чтобы она влезла в ширину w.
func ellipsize(face font.Face, s string, w float64) string {
	if measure(face, s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		t := strings.TrimRight(string(r), " ,.") + "…"
		if measure(face, t) <= w {
			return t
		}
	}
	return "…"
}

// metrics — высота заглавных букв (для вертикального центрирования).
func capHeight(face font.Face) float64 {
	if face == nil {
		return 0
	}
	b, _, ok := face.GlyphBounds('Н')
	if !ok {
		return float64(face.Metrics().Ascent) / 64 * 0.7
	}
	return float64(-b.Min.Y) / 64
}

// textCentered — строка по центру прямоугольника.
func textCentered(dst *image.RGBA, face font.Face, s string, r rect, c color.Color) {
	w := measure(face, s)
	text(dst, face, s, r.x+(r.w-w)/2, r.y+(r.h+capHeight(face))/2, c)
}

// firstRune — первый значимый символ эмодзи (без вариационных селекторов).
func firstRune(e string) rune {
	for len(e) > 0 {
		r, n := utf8.DecodeRuneInString(e)
		if r != 0xFE0F && r != 0x200D {
			return r
		}
		e = e[n:]
	}
	return 0
}

// drawImageFit вписывает картинку в прямоугольник (качественное масштабирование).
func drawImageFit(dst *image.RGBA, img image.Image, r rect) {
	xdraw.CatmullRom.Scale(dst, r.image(), img, img.Bounds(), xdraw.Over, nil)
}

func rgba(hex string, def color.NRGBA) color.NRGBA {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return def
	}
	var v [3]uint8
	for i := 0; i < 3; i++ {
		var x uint8
		for _, ch := range h[2*i : 2*i+2] {
			x <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				x |= uint8(ch - '0')
			case ch >= 'a' && ch <= 'f':
				x |= uint8(ch - 'a' + 10)
			case ch >= 'A' && ch <= 'F':
				x |= uint8(ch - 'A' + 10)
			default:
				return def
			}
		}
		v[i] = x
	}
	return color.NRGBA{v[0], v[1], v[2], 255}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }
