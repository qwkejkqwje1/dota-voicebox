package overlay

import (
	"fmt"
	"strings"
	"testing"
)

func readerSrc() *fakeSrc {
	src := &fakeSrc{canSay: true, voice: "vile", readHist: []string{"Иду на мид", "gg wp"}, search: []Item{
		{Kind: "sound", ID: "gank", Label: "Мид, ганк!", Emoji: "🚨", Color: "#ef4444", Hint: "Num1"},
		{Kind: "sound", ID: "mid2", Label: "Пропал мид", Emoji: "❓", Color: "#f59e0b"},
	}}
	for _, v := range [][3]string{{"sly", "😏", "Ехидный"}, {"vile", "🤢", "Мерзкий"}, {"demon", "👹", "Демон"},
		{"robot", "🤖", "Робот"}, {"chipmunk", "🐿️", "Бурундук"}, {"grandpa", "👴", "Дед"}, {"drunk", "🥴", "Пьяный"},
		{"bass", "🗿", "Басом"}, {"alien", "👽", "Пришелец"}, {"radio", "📻", "Рация"}} {
		src.voices = append(src.voices, Item{ID: v[0], Emoji: v[1], Label: v[2], Find: "описание " + v[2]})
	}
	return src
}

func openReader(c *Controller) {
	c.Open()
	c.StartReader()
}

func TestReaderKeys(t *testing.T) {
	src := readerSrc()
	c := &Controller{Src: src}
	c.StartReader() // меню закрыто — ничего
	if c.Reading() {
		t.Fatal("«Читалка» без меню")
	}
	openReader(c)
	v := c.View(1)
	if !c.Reading() || v.Badge != "Мерзкий" || v.BadgeEmoji != "🤢" || len(v.Items) != 2 || v.Items[0].Kind != "readhist" ||
		v.Hover != 0 || v.Items[0].Hint != "Enter — ещё раз" || v.Items[1].Hint != "" || !strings.Contains(v.Footer, "← →") {
		t.Fatalf("пустая строка — недавние фразы: %+v", v)
	}
	key(c, VKRight)
	if v := c.View(1); v.Badge != "Демон" {
		t.Fatalf("→: %q", v.Badge)
	}
	key(c, VKLeft)
	key(c, VKLeft)
	key(c, VKLeft) // по кругу: sly → последний
	if v := c.View(1); v.Badge != "Рация" {
		t.Fatalf("← по кругу: %q", v.Badge)
	}
	input(c, KeyInput{VK: VKTab})
	key(c, VKRight) // → vile
	key(c, VKDown)  // без текста ↑↓ — по фразам
	if r := key(c, VKReturn); !r.Close || c.Visible() || fmt.Sprint(src.picks) != "[read:vile:gg wp]" {
		t.Fatalf("недавняя фраза ещё раз: %v", src.picks)
	}

	// с текстом строки — голоса, ↑↓ выбирают голос
	openReader(c)
	typeText(c, "ну привет")
	v = c.View(1)
	if len(v.Items) != searchRows || v.Items[v.Hover].ID != "vile" || v.Items[0].Kind != "voice" || v.Items[1].Hint != "описание Мерзкий" ||
		!strings.Contains(v.Footer, "Ctrl+Enter") {
		t.Fatalf("голоса: %+v", v)
	}
	key(c, VKDown)
	if v := c.View(1); v.Badge != "Демон" || v.Items[v.Hover].ID != "demon" {
		t.Fatalf("↓: %+v", v)
	}
	c.Wheel(-120)
	c.Wheel(-120)
	if v := c.View(1); v.Badge != "Бурундук" {
		t.Fatalf("колёсико: %q", v.Badge)
	}
	c.Wheel(120)
	if v := c.View(1); v.Badge != "Робот" {
		t.Fatalf("колёсико назад: %q", v.Badge)
	}
	key(c, VKPageDown) // в конец списка: окно строк сдвигается
	v = c.View(1)
	if v.Badge != "Рация" || v.Items[v.Hover].ID != "radio" || v.Items[len(v.Items)-1].ID != "radio" {
		t.Fatalf("PgDn: %+v", v)
	}
	key(c, VKPageUp)
	key(c, VKPageUp)
	if v := c.View(1); v.Badge != "Ехидный" || v.Hover != 0 {
		t.Fatalf("PgUp: %+v", v)
	}
	if c.HoverAt(3); c.View(1).Badge != "Робот" { // голос под мышью — выбранный
		t.Fatal("мышь над голосом")
	}
	if r := input(c, KeyInput{VK: VKReturn, Ctrl: true}); !r.Close || src.picks[1] != "readme:robot:ну привет" {
		t.Fatalf("Ctrl+Enter — себе: %v", src.picks)
	}

	// щелчок по голосу — прочитать им
	openReader(c)
	typeText(c, "gg")
	if r := c.Click("row", 2); r.Act == nil || !r.Close {
		t.Fatal("щелчок")
	} else {
		r.Act()
	}
	if src.picks[2] != "read:demon:gg" {
		t.Fatalf("щелчок по голосу: %v", src.picks)
	}

	// Esc закрывает меню целиком; Enter без текста и без фраз — ничего
	openReader(c)
	if r := key(c, VKEscape); !r.Close || c.Visible() {
		t.Fatal("Esc")
	}
	src.readHist = nil
	openReader(c)
	if r := key(c, VKReturn); r.Close || !c.Visible() || len(src.picks) != 3 {
		t.Fatal("Enter без текста")
	}
	if v := c.View(1); v.Empty == "" || len(v.Items) != 0 {
		t.Fatalf("подсказка: %+v", v)
	}
	// обычный поиск — после «Сказать» строка «Прочитать голосом»
	c.Open()
	c.StartTyping()
	typeText(c, "мид")
	v = c.View(1)
	last := v.Items[len(v.Items)-1]
	if c.Reading() || len(v.Items) != 4 || v.Items[2].Kind != "say" || last.Kind != "read:vile" || last.ID != "мид" ||
		!strings.Contains(last.Label, "«Мерзкий»") {
		t.Fatalf("поиск: %+v", v.Items)
	}
	if r := c.Click("row", 3); r.Act == nil || !r.Close {
		t.Fatal("«Прочитать голосом» закрывает меню")
	} else {
		r.Act()
	}
	if src.picks[3] != "read:vile:мид" {
		t.Fatalf("%v", src.picks)
	}
	// без синтеза речи — нечем
	src.canSay = false
	openReader(c)
	typeText(c, "мид")
	if v := c.View(1); !strings.Contains(v.Empty, "недоступны") {
		t.Fatalf("без голосов: %+v", v)
	}
	c.Open()
	c.StartTyping()
	typeText(c, "мид")
	for _, it := range c.View(1).Items {
		if strings.HasPrefix(it.Kind, "read") || it.Kind == "say" {
			t.Fatalf("без синтеза речи: %+v", it)
		}
	}
}

func TestReaderRender(t *testing.T) {
	r := &Renderer{F: LoadFonts()}
	src := readerSrc()
	c := &Controller{Src: src}
	openReader(c)
	img, l := r.Menu(c.View(1))
	if len(l.Rows) != 2 {
		t.Fatalf("недавние фразы: %+v", l)
	}
	save(t, "reader_empty.png", img)
	typeText(c, "Ты чего такой серьёзный?")
	key(c, VKDown)
	img, l = r.Menu(c.View(1.25))
	if len(l.Rows) != searchRows {
		t.Fatalf("голоса: %+v", l)
	}
	save(t, "reader_typed.png", img)
	src.readHist = nil
	openReader(c)
	img, _ = r.Menu(c.View(1))
	save(t, "reader_new.png", img)
	c.Open()
	c.StartTyping()
	typeText(c, "мид")
	img, l = r.Menu(c.View(1))
	if len(l.Rows) != 4 {
		t.Fatalf("поиск: %+v", l)
	}
	save(t, "search_read.png", img)
}
