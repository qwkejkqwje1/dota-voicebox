package overlay

import (
	"image"
	"testing"
)

func center(r image.Rectangle) image.Point { return r.Min.Add(r.Size().Div(2)) }

func checkRows(t *testing.T, l Layout, n int) {
	t.Helper()
	if len(l.Rows) != n {
		t.Fatalf("плиток %d, нужно %d", len(l.Rows), n)
	}
	for i, r := range l.Rows {
		c := center(r)
		if k, idx := l.HitTest(c.X, c.Y); k != "row" || idx != i {
			t.Fatalf("плитка %d: %s %d", i, k, idx)
		}
	}
}

func TestStripRender(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	for _, sc := range []float64{1, 1.5} {
		v := sampleView(sc)
		v.Layout, v.Slots, v.Hover = LayoutStrip, 10, 3
		img, l := r.Menu(v)
		checkRows(t, l, 10)
		if len(l.Tabs) != 2 || l.Prev.Empty() || l.Next.Empty() || l.Grip.Empty() {
			t.Fatalf("%+v", l)
		}
		for name, want := range map[string]image.Rectangle{"prev": l.Prev, "next": l.Next, "grip": l.Grip} {
			c := center(want)
			if k, _ := l.HitTest(c.X, c.Y); k != name {
				t.Fatalf("%s: %s", name, k)
			}
		}
		// между плитками — фон меню: за него можно тащить
		gap := image.Pt((l.Rows[0].Max.X+l.Rows[1].Min.X)/2, center(l.Rows[0]).Y)
		if k, _ := l.HitTest(gap.X, gap.Y); k != "panel" {
			t.Fatalf("между плитками: %s", k)
		}
		if l.H > l.W/5 {
			t.Fatalf("полоска должна быть низкой: %dx%d", l.W, l.H)
		}
		save(t, "strip_"+itoa(int(sc*10))+".png", img)
	}
	// узкий экран — полоска ужимается
	v := sampleView(1.5)
	v.Layout, v.MaxW = LayoutStrip, 1200
	if _, l := r.Menu(v); l.W > 1200 {
		t.Fatalf("не влезла: %d", l.W)
	}
	// одна страница, одна вкладка, 4 звука: без стрелок и вкладок, лишние места пустые
	v = sampleView(1)
	v.Layout, v.Tabs, v.Pages, v.Items, v.Slots, v.Opacity = LayoutStrip, []string{"Звуки"}, 1, v.Items[:4], 6, 0.6
	img, l := r.Menu(v)
	checkRows(t, l, 4)
	if len(l.Tabs) != 0 || !l.Prev.Empty() {
		t.Fatalf("%+v", l)
	}
	save(t, "strip_small.png", img)
	empty, _ := r.Menu(MenuView{Layout: LayoutStrip, Tabs: []string{"Звуки"}, Hover: -1, Pages: 1})
	save(t, "strip_empty.png", empty)
}

func TestGridRender(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	v := sampleView(1.25)
	v.Layout, v.Slots, v.Hover = LayoutGrid, 10, 6
	img, l := r.Menu(v)
	checkRows(t, l, 10)
	if len(l.Tabs) != 2 || l.Grip.Empty() {
		t.Fatalf("%+v", l)
	}
	if l.Rows[5].Min.Y <= l.Rows[4].Max.Y || l.Rows[5].Min.X != l.Rows[0].Min.X {
		t.Fatal("должно быть два ряда по 5")
	}
	save(t, "grid.png", img)
	v = sampleView(1)
	v.Layout, v.Items, v.Slots, v.Pages = LayoutGrid, v.Items[:3], 3, 1
	img, l = r.Menu(v)
	checkRows(t, l, 3)
	save(t, "grid_small.png", img)
}

func TestListGrip(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	v := sampleView(1)
	v.Opacity = 0.7
	img, l := r.Menu(v)
	c := center(l.Grip)
	if k, _ := l.HitTest(c.X, c.Y); k != "grip" || len(l.Tabs) != 2 {
		t.Fatal(k, l.Tabs)
	}
	if l.Tabs[1].Max.X >= l.Grip.Min.X {
		t.Fatal("вкладки налезают на ручку")
	}
	save(t, "list_grip.png", img)
}
