package overlay

import (
	"image"
	"math"
	"os"
	"testing"
	"time"
)

func sampleWheel() []WheelItem {
	return []WheelItem{
		{Act: "sound:airhorn", Emoji: "📯", Label: "Дудка", Color: "#f59e0b"},
		{Act: "sound:sad", Emoji: "🎺", Label: "Грустный тромбон", Color: "#60a5fa", Active: true},
		{Act: "sound:rim", Emoji: "🥁", Label: "Ба-дум-тсс", Color: "#a78bfa"},
		{Act: "sound:gank", Emoji: "🚨", Label: "Ганк на миде! Очень длинное название", Color: "#ef4444"},
		{Act: "stop", Emoji: "⏹️", Label: "Стоп", Color: "#ef4444"},
		{},
		{Act: "preset:radio", Emoji: "📻", Label: "Рация", Color: "#8b5cf6"},
		{Act: "sound:hi", Emoji: "🗣️", Label: "Всем привет", Color: "#06b6d4", Pending: true},
	}
}

type wheelSrc struct{ picked []string }

func (w *wheelSrc) WheelData() WheelData {
	return WheelData{Items: sampleWheel(), Scale: 1, Opacity: 0.92}
}
func (w *wheelSrc) WheelPick(act string) { w.picked = append(w.picked, act) }

func TestWheelRender(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	if dir := os.Getenv("OVERLAY_EMOJI"); dir != "" { // цветные эмодзи, как в программе (кэш интерфейса)
		r.Emoji = &FileEmoji{Dir: dir}
	}
	cases := []struct {
		name string
		v    WheelView
	}{
		{"wheel-idle.png", WheelView{Items: sampleWheel(), Hover: -1, Scale: 1}},
		{"wheel-hover0.png", WheelView{Items: sampleWheel(), Hover: 0, PointerOn: true, Pointer: 0.1, Scale: 1}},
		{"wheel-hover1.png", WheelView{Items: sampleWheel(), Hover: 1, PointerOn: true, Pointer: math.Pi / 4, Scale: 1}},
		{"wheel-hover3.png", WheelView{Items: sampleWheel(), Hover: 3, PointerOn: true, Pointer: 3 * math.Pi / 4, Scale: 1, Opacity: 0.5}},
		{"wheel-sticky.png", WheelView{Items: sampleWheel(), Hover: -1, Sticky: true, Scale: 1}},
		{"wheel-hover6-15.png", WheelView{Items: sampleWheel(), Hover: 6, PointerOn: true, Pointer: 3 * math.Pi / 2, Sticky: true, Scale: 1.5}},
		{"wheel-small.png", WheelView{Items: sampleWheel(), Hover: 2, PointerOn: true, Pointer: math.Pi / 2, Scale: 0.7}},
	}
	for _, c := range cases {
		t0 := time.Now()
		img := r.Wheel(c.v)
		el := time.Since(t0)
		t.Logf("%s: %dx%d за %v", c.name, img.Bounds().Dx(), img.Bounds().Dy(), el)
		if want := WheelSize(c.v.Scale); img.Bounds().Dx() != want || img.Bounds().Dy() != want {
			t.Fatalf("%s: размер %v, ждали %d", c.name, img.Bounds(), want)
		}
		if el > 300*time.Millisecond {
			t.Errorf("%s: слишком медленно: %v", c.name, el)
		}
		save(t, c.name, img)
	}
}

func TestWheelSlots(t *testing.T) {
	for _, c := range []struct {
		dx, dy float64
		want   int
	}{{0, -10, 0}, {10, -10, 1}, {10, 0, 2}, {10, 10, 3}, {0, 10, 4}, {-10, 10, 5}, {-10, 0, 6}, {-10, -10, 7}, {-1, -10, 0}, {-5, -10, 7}} {
		if got := slotAt(direction(c.dx, c.dy)); got != c.want {
			t.Errorf("(%v,%v): сектор %d, ждали %d", c.dx, c.dy, got, c.want)
		}
	}
}

func TestWheelHold(t *testing.T) {
	src := &wheelSrc{}
	w := &WheelCtl{Src: src}
	now := time.Now()
	o := image.Pt(500, 400)
	w.Open(src.WheelData(), 0x51, true, o, o, 1, now)
	if !w.Visible() || w.Sticky() {
		t.Fatal("должно открыться в режиме удержания")
	}
	// мелкая дрожь руки — ничего не выбрано
	w.Move(o.Add(image.Pt(5, -8)), now.Add(50*time.Millisecond))
	if v := w.View(); v.Hover != -1 {
		t.Fatalf("дрожь выбрала сектор %d", v.Hover)
	}
	// вправо — сектор 3 (снизу справа) … нет: вправо = 2
	if !w.Move(o.Add(image.Pt(80, 2)), now.Add(100*time.Millisecond)) {
		t.Fatal("ждали перерисовку")
	}
	if v := w.View(); v.Hover != 2 || !v.PointerOn {
		t.Fatalf("ждали сектор 2: %+v", v.Hover)
	}
	// пустой сектор (5: снизу слева) — без выбора, но стрелка есть
	w.Move(o.Add(image.Pt(-60, 60)), now.Add(150*time.Millisecond))
	if v := w.View(); v.Hover != -1 || !v.PointerOn {
		t.Fatalf("пустой сектор: %+v", v.Hover)
	}
	w.Move(o.Add(image.Pt(0, -90)), now.Add(200*time.Millisecond))
	r := w.Key(0x51, true, now.Add(220*time.Millisecond)) // автоповтор
	if !r.Handled || r.Close {
		t.Fatalf("автоповтор: %+v", r)
	}
	r = w.Key(0x51, false, now.Add(600*time.Millisecond))
	if !r.Close || r.Act == nil || r.Handled {
		t.Fatalf("отпускание: %+v", r)
	}
	r.Act()
	if len(src.picked) != 1 || src.picked[0] != "sound:airhorn" || w.Visible() {
		t.Fatalf("выбор: %v", src.picked)
	}
}

func TestWheelCancelAndTap(t *testing.T) {
	src := &wheelSrc{}
	w := &WheelCtl{Src: src}
	now := time.Now()
	o := image.Pt(500, 400)
	// держали долго и отпустили в середине — отмена
	w.Open(src.WheelData(), 0x05, true, o, o, 1, now)
	if r := w.Release(now.Add(time.Second)); !r.Close || r.Act != nil || w.Visible() {
		t.Fatalf("отмена: %+v", r)
	}
	// быстро нажали — колесо остаётся, выбор щелчком
	w.Open(src.WheelData(), 0x05, true, o, o, 1, now)
	if r := w.Release(now.Add(120 * time.Millisecond)); r.Close || !w.Sticky() || !w.Visible() {
		t.Fatalf("быстрое нажатие: %+v", r)
	}
	// в «залипшем» колесе выбор — по месту курсора: середина ничего не выбирает
	if r := w.Click(o.Add(image.Pt(10, 10)), now.Add(time.Second)); !r.Close || r.Act != nil {
		t.Fatalf("щелчок в середине: %+v", r)
	}
	w.Open(src.WheelData(), 0x05, true, o, o, 1, now)
	w.Release(now.Add(100 * time.Millisecond))
	r := w.Click(o.Add(image.Pt(120, 0)), now.Add(time.Second))
	if !r.Close || r.Act == nil || !r.Handled {
		t.Fatalf("щелчок по сектору: %+v", r)
	}
	r.Act()
	if src.picked[len(src.picked)-1] != "sound:rim" {
		t.Fatalf("выбор щелчком: %v", src.picked)
	}
	// открыто без клавиши (из интерфейса) — сразу щелчком; цифра выбирает сектор
	w.Open(src.WheelData(), 0, false, o, image.Pt(960, 540), 1, now)
	if !w.Sticky() {
		t.Fatal("без клавиши — сразу щелчком")
	}
	if r := w.Key(0x36, true, now); !r.Handled || r.Act != nil { // 6 — пустой сектор
		t.Fatalf("пустой сектор по цифре: %+v", r)
	}
	if r := w.Key(0x65, true, now); !r.Handled || r.Act == nil || !r.Close { // Num5 — «Стоп»
		t.Fatalf("Num5: %+v", r)
	}
	// чужая клавиша закрывает «залипшее» колесо и уходит игре
	w.Open(src.WheelData(), 0, false, o, o, 1, now)
	if r := w.Key(0x41, true, now); r.Handled || !r.Close {
		t.Fatalf("чужая клавиша: %+v", r)
	}
	// само закрывается после бездействия
	w.Open(src.WheelData(), 0, false, o, o, 1, now)
	if r := w.Tick(o, false, now.Add(WheelIdle+time.Second)); !r.Close || w.Visible() {
		t.Fatalf("бездействие: %+v", r)
	}
	// отпускание, замеченное опросом (перехватчик пропустил событие)
	w.Open(src.WheelData(), 0x51, true, o, o, 1, now)
	w.Move(o.Add(image.Pt(-70, 0)), now.Add(100*time.Millisecond))
	r = w.Tick(o.Add(image.Pt(-70, 0)), false, now.Add(400*time.Millisecond))
	if !r.Close || r.Act == nil {
		t.Fatalf("опрос: %+v", r)
	}
	r.Act()
	if src.picked[len(src.picked)-1] != "preset:radio" {
		t.Fatalf("опрос — выбор: %v", src.picked)
	}
	// клавишу отпустили ещё до открытия окна — сразу выбор щелчком
	w.Open(src.WheelData(), 0x51, false, o, o, 1, now)
	if !w.Sticky() {
		t.Fatal("клавиша уже отпущена — ждали выбор щелчком")
	}
	// повторное нажатие клавиши колеса закрывает «залипшее» колесо (и не открывает снова)
	if r := w.Key(0x51, true, now); !r.Handled || !r.Close || w.Visible() {
		t.Fatalf("повторное нажатие: %+v", r)
	}
}
