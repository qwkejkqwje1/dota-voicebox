package overlay

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
)

func sampleView(scale float64) MenuView {
	return MenuView{
		Tabs: []string{"Звуки", "Голос"}, Tab: 0, Page: 0, Pages: 2, Hover: 2, Scale: scale,
		Footer: Footer(LayoutList, 2, true),
		Items: []Item{
			{Key: "1", Emoji: "📯", Label: "Дудка", Color: "#f59e0b", Hint: "Ctrl+1"},
			{Key: "2", Emoji: "🎺", Label: "Грустный тромбон", Color: "#60a5fa", Active: true},
			{Key: "3", Emoji: "🥁", Label: "Ба-дум-тсс", Color: "#a78bfa"},
			{Key: "4", Emoji: "🦗", Label: "Сверчки", Color: "#34d399"},
			{Key: "5", Emoji: "✅", Label: "Правильно!", Color: "#22c55e"},
			{Key: "6", Emoji: "❌", Label: "Неправильно!", Color: "#ef4444"},
			{Key: "7", Emoji: "🚨", Label: "Ганк на миде! Очень длинное название, которое не влезет", Color: "#ef4444", Hint: "Num1"},
			{Key: "8", Emoji: "🗣️", Label: "Всем привет", Color: "#06b6d4", Pending: true},
			{Key: "9", Emoji: "", Label: "мой звук", Color: "#ec4899"},
			{Key: "0", Emoji: "🔫", Label: "Пиу-пиу", Color: "#22d3ee"},
		},
	}
}

// OVERLAY_LANG=en — картинки (OVERLAY_PNG) с английским интерфейсом.
func TestMain(m *testing.M) {
	if os.Getenv("OVERLAY_LANG") == "en" {
		i18n.SetLang("en")
	}
	os.Exit(m.Run())
}

func save(t *testing.T, name string, img image.Image) {
	dir := os.Getenv("OVERLAY_PNG")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}

func TestMenuRender(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	for _, sc := range []float64{1, 1.5} {
		t0 := time.Now()
		img, l := r.Menu(sampleView(sc))
		el := time.Since(t0)
		t.Logf("масштаб %.1f: %dx%d за %v", sc, l.W, l.H, el)
		if el > 400*time.Millisecond {
			t.Errorf("слишком медленно: %v", el)
		}
		w, h := MenuSize(10, true, sc)
		if l.W != w || l.H != h || img.Bounds().Dx() != w {
			t.Fatalf("размер %dx%d, MenuSize %dx%d", l.W, l.H, w, h)
		}
		if len(l.Rows) != 10 || len(l.Tabs) != 2 {
			t.Fatalf("%+v", l)
		}
		for i, row := range l.Rows {
			c := row.Min.Add(row.Size().Div(2))
			if k, idx := l.HitTest(c.X, c.Y); k != "row" || idx != i {
				t.Fatalf("строка %d: %s %d", i, k, idx)
			}
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Fatal("угол должен быть прозрачным")
		}
		c := l.Panel.Min.Add(image.Pt(5, l.Panel.Dy()/2))
		if _, _, _, a := img.At(c.X, c.Y).RGBA(); a < 0xE000 {
			t.Fatalf("фон меню прозрачный: %x", a)
		}
		save(t, "menu_"+itoa(int(sc*10))+".png", img)
	}
	empty, _ := r.Menu(MenuView{Tabs: []string{"Звуки"}, Hover: -1})
	save(t, "menu_empty.png", empty)
}

func TestToastRender(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	img := r.Toast(ToastView{Emoji: "📻", Text: "Рация", Sub: "эффект голоса", Scale: 1.25})
	if img.Bounds().Dx() < 200 {
		t.Fatal(img.Bounds())
	}
	save(t, "toast.png", img)
	save(t, "toast2.png", r.Toast(ToastView{Emoji: "⏰", Text: "Руна мудрости через 20 секунд", Scale: 1}))
}

func TestSearchRender(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	src := &fakeSrc{canSay: true, phrases: []string{"Иду на мид", "Нужен вард у рошана"}, search: []Item{
		{Kind: "sound", ID: "gank", Label: "Мид, ганк!", Emoji: "🚨", Color: "#ef4444", Hint: "Num1"},
		{Kind: "sound", ID: "mid2", Label: "Пропал мид", Emoji: "❓", Color: "#f59e0b"},
		{Kind: "effect", ID: "radio", Label: "Рация", Emoji: "📻", Color: "#8b5cf6", Hint: "эффект"},
	}}
	c := &Controller{Src: src}
	c.Open()
	c.StartTyping()
	img, l := r.Menu(c.View(1))
	if len(l.Rows) != 2 || l.Grip.Empty() {
		t.Fatalf("недавние фразы: %+v", l)
	}
	save(t, "search_empty.png", img)
	typeText(c, "мид")
	img, l = r.Menu(c.View(1.25))
	if len(l.Rows) != 3 {
		t.Fatalf("%+v", l)
	}
	save(t, "search_mid.png", img)
	typeText(c, " очень длинный текст, который не помещается в строку поиска целиком")
	img, _ = r.Menu(c.View(1))
	save(t, "search_long.png", img)
	c.Open()
	img, _ = r.Menu(c.View(1))
	save(t, "menu_footer.png", img)
}
