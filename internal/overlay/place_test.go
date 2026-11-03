package overlay

import (
	"image"
	"testing"
)

func TestPlace(t *testing.T) {
	area := image.Rect(0, 0, 1920, 1080)
	panel := image.Rect(18, 18, 382, 282) // картинка 400×300, вокруг панели поля под тень
	w, h := 400, 300
	cases := []struct {
		p    Placement
		want image.Point
	}{
		{Placement{Position: "center"}, image.Pt(760, 390)},
		{Placement{Position: "top"}, image.Pt(760, 86)},
		{Placement{Position: "bottom"}, image.Pt(760, 1080-300-216)},
		{Placement{Position: "top-right"}, image.Pt(1920-400-38, 86)},
		{Placement{Position: "bottom-left"}, image.Pt(38, 564)},
		{Placement{Position: "custom", X: 0, Y: 0}, image.Pt(-18, -18)},           // панель вплотную к углу
		{Placement{Position: "custom", X: 1, Y: 1}, image.Pt(1920-382, 1080-282)}, //
		{Placement{Position: "custom", X: 0.5, Y: 0}, image.Pt(760, -18)},         // полоска сверху по центру
		{Placement{Position: "cursor", Cursor: image.Pt(5, 5), Scale: 1}, image.Pt(-18, -13)},
		{Placement{Position: "nowhere"}, image.Pt(760, 390)},
	}
	for _, c := range cases {
		if got := Place(area, w, h, panel, c.p); got != c.want {
			t.Errorf("%+v: %v, нужно %v", c.p, got, c.want)
		}
	}
	// обратно: место после перетаскивания
	for _, o := range []image.Point{{-18, -18}, {778, 400}, {1538, 798}, {100, 7}} {
		x, y := CustomXY(area, o, panel)
		if got := Place(area, w, h, panel, Placement{Position: "custom", X: x, Y: y}); abs(got.X-o.X) > 2 || abs(got.Y-o.Y) > 2 {
			t.Errorf("%v → (%.3f, %.3f) → %v", o, x, y, got)
		}
	}
	// второй монитор слева (отрицательные координаты)
	left := image.Rect(-1920, 0, 0, 1080)
	if got := Place(left, w, h, panel, Placement{Position: "center"}); got != image.Pt(-1160, 390) {
		t.Error(got)
	}
	if got := Clamp(area, image.Pt(5000, -400), panel); got != image.Pt(1538, -18) {
		t.Error(got)
	}
}

func TestAlignReanchor(t *testing.T) {
	for _, c := range []struct {
		p      Placement
		ax, ay float64
	}{
		{Placement{Position: "center"}, 0.5, 0},
		{Placement{Position: "bottom-right"}, 1, 1},
		{Placement{Position: "left"}, 0, 0},
		{Placement{Position: "custom", X: 0.1, Y: 0.9}, 0, 1},
		{Placement{Position: "custom", X: 0.5, Y: 0.5}, 0.5, 0},
	} {
		if ax, ay := Align(c.p); ax != c.ax || ay != c.ay {
			t.Errorf("%+v: %v %v", c.p, ax, ay)
		}
	}
	// снизу: стало меньше строк — низ на месте
	if got := Reanchor(image.Pt(100, 500), image.Pt(400, 300), image.Pt(400, 200), 0.5, 1); got != image.Pt(100, 600) {
		t.Error(got)
	}
	// полоска по центру стала уже — середина на месте
	if got := Reanchor(image.Pt(100, 0), image.Pt(1000, 90), image.Pt(800, 90), 0.5, 0); got != image.Pt(200, 0) {
		t.Error(got)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
