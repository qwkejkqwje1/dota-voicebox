package overlay

import (
	"fmt"
	"testing"
	"time"
)

type fakeSrc struct {
	sounds, effects int
	picks           []string
	stops           int
	timeout         int
	closeAfter      bool
	search          []Item
	phrases         []string
	canSay          bool
}

func (f *fakeSrc) MenuData() Data {
	d := Data{TimeoutSec: f.timeout, CloseAfterPick: f.closeAfter, Search: f.search, Phrases: f.phrases, CanSay: f.canSay}
	for i := 0; i < f.sounds; i++ {
		d.Sounds = append(d.Sounds, Item{ID: fmt.Sprintf("s%d", i), Label: fmt.Sprintf("Звук %d", i)})
	}
	for i := 0; i < f.effects; i++ {
		d.Effects = append(d.Effects, Item{ID: fmt.Sprintf("e%d", i), Label: fmt.Sprintf("Эффект %d", i)})
	}
	return d
}
func (f *fakeSrc) Pick(kind, id string) { f.picks = append(f.picks, kind+":"+id) }
func (f *fakeSrc) StopAll()             { f.stops++ }

// key — нажатие с выполнением действия (как делает окно меню).
func key(c *Controller, vk uint32) Result {
	r := c.Key(vk)
	if r.Act != nil {
		r.Act()
	}
	return r
}

func TestControllerKeys(t *testing.T) {
	src := &fakeSrc{sounds: 13, effects: 4}
	c := &Controller{Src: src}
	if r := key(c, '1'); r.Handled {
		t.Fatal("закрытое меню не должно брать клавиши")
	}
	c.Open()
	v := c.View(1)
	if len(v.Items) != 10 || v.Pages != 2 || v.Items[9].Key != "0" || len(v.Tabs) != 2 {
		t.Fatalf("%+v", v)
	}
	if r := key(c, 0x33); !r.Handled || r.Close { // «3»
		t.Fatalf("%+v", r)
	}
	if r := key(c, 0x60); !r.Handled { // Num0 → 10-й
		t.Fatal(r)
	}
	key(c, VKRight)                    // страница 2: s10..s12
	key(c, 0x62)                       // Num2 → s11
	if r := key(c, 0x39); !r.Handled { // «9» — пусто на этой странице
		t.Fatal(r)
	}
	key(c, VKTab)
	key(c, 0x31)     // эффект e0 — выделение остаётся на нём
	key(c, VKDown)   // e1
	key(c, VKReturn) // e1
	key(c, VKUp)
	key(c, VKUp) // с e0 по кругу на последний — e3
	key(c, VKReturn)
	want := []string{"sound:s2", "sound:s9", "sound:s11", "effect:e0", "effect:e1", "effect:e3"}
	if fmt.Sprint(src.picks) != fmt.Sprint(want) {
		t.Fatalf("%v", src.picks)
	}
	if r := key(c, 0x10); r.Handled || r.Close { // Shift — не трогаем
		t.Fatal(r)
	}
	if r := key(c, 'Q'); r.Handled || !r.Close || c.Visible() { // другая клавиша закрывает меню и уходит в игру
		t.Fatal(r)
	}
	c.Open()
	if c.View(1).Tab != 0 {
		t.Fatal("меню открывается на звуках")
	}
	if r := key(c, VKEscape); !r.Close || c.Visible() {
		t.Fatal(r)
	}
}

func TestControllerCloseAfterPickTimeoutAndMouse(t *testing.T) {
	src := &fakeSrc{sounds: 3, closeAfter: true, timeout: 1}
	c := &Controller{Src: src}
	c.Open()
	if len(c.View(1).Tabs) != 1 {
		t.Fatal("без эффектов — одна вкладка")
	}
	if c.HoverAt(1) != true || c.HoverAt(1) != false {
		t.Fatal("hover")
	}
	r := c.Click("row", 2)
	if r.Act == nil {
		t.Fatal("нет действия")
	}
	r.Act()
	if !r.Close || c.Visible() || src.picks[0] != "sound:s2" {
		t.Fatal(r, src.picks)
	}
	c.Open()
	if c.Expired(time.Now()) || !c.Expired(time.Now().Add(2*time.Second)) {
		t.Fatal("таймаут")
	}
	src.sounds = 0
	c.Refresh()
	if v := c.View(1); v.Empty == "" || len(v.Items) != 0 {
		t.Fatalf("%+v", v)
	}
}

func TestControllerWheel(t *testing.T) {
	src := &fakeSrc{sounds: 25}
	c := &Controller{Src: src}
	if r := c.Wheel(-120); r.Handled {
		t.Fatal("закрытое меню")
	}
	c.Open()
	c.Wheel(-120)
	c.Wheel(-120)
	c.Wheel(-120) // дальше последней не листает
	if v := c.View(1); v.Page != 2 || len(v.Items) != 5 {
		t.Fatalf("%+v", v)
	}
	c.Wheel(120)
	if c.View(1).Page != 1 {
		t.Fatal("назад")
	}
}

func TestControllerStripGrid(t *testing.T) {
	src := &fakeSrc{sounds: 13, effects: 4}
	c := &Controller{Src: src}
	c.Open()
	c.mu.Lock()
	c.data.Layout = LayoutStrip
	c.mu.Unlock()
	if v := c.View(1); v.Slots != 10 || v.Footer == "" {
		t.Fatalf("%+v", v)
	}
	key(c, VKRight) // выделение на первую плитку
	if v := c.View(1); v.Hover != 0 {
		t.Fatal(v.Hover)
	}
	key(c, VKLeft) // с первой — на последнюю плитку прошлой страницы (по кругу)
	if v := c.View(1); v.Page != 1 || v.Hover != 2 {
		t.Fatalf("стр %d, плитка %d", v.Page, v.Hover)
	}
	key(c, VKReturn)
	key(c, VKDown) // полоска: ↓ — следующая страница
	if v := c.View(1); v.Page != 0 || v.Hover != -1 {
		t.Fatalf("%+v", v)
	}
	if r := c.Click("next", -1); !r.Redraw || c.View(1).Page != 1 {
		t.Fatal("стрелка ▼")
	}
	c.Click("prev", -1)
	key(c, VKTab)
	if v := c.View(1); v.Slots != 10 || len(v.Items) != 4 {
		t.Fatalf("размер полоски не должен меняться: %+v", v)
	}
	key(c, VKTab)

	c.mu.Lock()
	c.data.Layout = LayoutGrid
	c.mu.Unlock()
	key(c, VKRight) // 0
	key(c, VKDown)  // 5
	key(c, VKDown)  // ниже некуда
	key(c, VKRight) // 6
	if v := c.View(1); v.Hover != 6 {
		t.Fatal(v.Hover)
	}
	key(c, VKUp) // 1
	key(c, VKReturn)
	key(c, VKPageDown) // страница 2: s10..s12
	key(c, VKRight)
	key(c, VKDown) // во втором ряду пусто — остаёмся
	key(c, VKReturn)
	want := []string{"sound:s12", "sound:s1", "sound:s10"}
	if fmt.Sprint(src.picks) != fmt.Sprint(want) {
		t.Fatalf("%v", src.picks)
	}
}

func TestFooter(t *testing.T) {
	for _, c := range []struct {
		layout  string
		pages   int
		effects bool
		want    string
	}{
		{LayoutList, 1, false, "1–0 выбор  ·  Enter поиск  ·  Backspace стоп  ·  Esc"},
		{LayoutList, 1, true, "1–0 выбор  ·  Tab голос  ·  Enter поиск  ·  Esc"},
		{LayoutList, 2, true, "1–0 выбор  ·  ← → стр.  ·  Tab голос  ·  Enter поиск  ·  Esc"},
		{LayoutGrid, 2, false, "1–0 выбор  ·  PgUp PgDn стр.  ·  Enter поиск  ·  Esc"},
	} {
		if got := Footer(c.layout, c.pages, c.effects); got != c.want {
			t.Errorf("%+v: %q", c, got)
		}
	}
}

// typeText — набрать строку (как делает окно меню: символ на каждую клавишу).
func typeText(c *Controller, s string) {
	for _, r := range s {
		vk := uint32(0x41)
		if r == ' ' {
			vk = VKSpace
		}
		c.Input(KeyInput{VK: vk, Text: string(r)})
	}
}

func input(c *Controller, k KeyInput) Result {
	r := c.Input(k)
	if r.Act != nil {
		r.Act()
	}
	return r
}

func TestControllerTyping(t *testing.T) {
	src := &fakeSrc{sounds: 3, effects: 2, canSay: true, phrases: []string{"Иду на мид"}, search: []Item{
		{Kind: "sound", ID: "gank", Label: "Мид, ганк!", Find: "dota"},
		{Kind: "sound", ID: "smoke", Label: "Смоук", Find: "dota"},
		{Kind: "sound", ID: "hello", Label: "Всем привет", Find: "фраза"},
		{Kind: "effect", ID: "radio", Label: "Рация"},
		{Kind: "sound", ID: "mid2", Label: "Пропал мид"},
	}}
	c := &Controller{Src: src}
	c.Open()
	if r := key(c, VKReturn); !r.Handled || !c.Typing() {
		t.Fatal("Enter без выделения — строка поиска")
	}
	v := c.View(1)
	if !v.Typing || v.Layout != LayoutList || len(v.Items) != 1 || v.Items[0].Kind != "say" || v.Items[0].ID != "Иду на мид" {
		t.Fatalf("пустой запрос — недавние фразы: %+v", v.Items)
	}
	// цифры и буквы — в строку, а не выбор и не закрытие меню
	typeText(c, "мид 2")
	if v := c.View(1); v.Query != "мид 2" || len(v.Items) != 1 || v.Items[0].Kind != "say" {
		t.Fatalf("«мид 2»: %+v", v.Items)
	}
	key(c, VKBack)
	key(c, VKBack)
	v = c.View(1)
	// «мид»: сначала то, что начинается с запроса, потом остальное; последней — «Сказать команде»
	if len(v.Items) != 3 || v.Items[0].ID != "gank" || v.Items[1].ID != "mid2" || v.Items[2].Kind != "say" || v.Items[2].ID != "мид" {
		t.Fatalf("«мид»: %+v", v.Items)
	}
	if v.Hover != 0 || v.Items[0].Hint != "Enter" || v.Items[0].Key != "" {
		t.Fatalf("выделение: %+v", v)
	}
	if r := key(c, 0x10); r.Handled { // Shift уходит дальше (иначе Windows не узнает, что он зажат)
		t.Fatal("Shift съеден")
	}
	if r := key(c, 0x70); !r.Handled || !c.Visible() { // F1 и прочее — не в игру, пока печатаем
		t.Fatal("F1 в режиме набора")
	}
	key(c, VKDown)
	key(c, VKReturn) // «Пропал мид»; меню остаётся, строка поиска закрывается
	if fmt.Sprint(src.picks) != "[sound:mid2]" || c.Typing() || !c.Visible() {
		t.Fatalf("%v typing=%v", src.picks, c.Typing())
	}
	// не та раскладка: «hfwbz» → «рация»
	key(c, VKReturn)
	typeText(c, "hfwbz")
	if v := c.View(1); v.Items[0].ID != "radio" || v.Items[0].Kind != "effect" {
		t.Fatalf("раскладка: %+v", v.Items)
	}
	key(c, VKReturn)
	// Ctrl+Enter — сказать набранное, что бы ни было выбрано; меню закрывается
	key(c, VKReturn)
	typeText(c, "Всем")
	if r := input(c, KeyInput{VK: VKReturn, Ctrl: true}); !r.Close || c.Visible() {
		t.Fatal("после фразы меню закрывается")
	}
	want := "[sound:mid2 effect:radio say:Всем]"
	if fmt.Sprint(src.picks) != want {
		t.Fatalf("%v", src.picks)
	}
	// Esc — назад к меню, второй Esc закрывает; Ctrl+Backspace стирает всё
	c.Open()
	key(c, VKReturn)
	typeText(c, "ганк")
	input(c, KeyInput{VK: VKBack, Ctrl: true})
	if c.View(1).Query != "" {
		t.Fatal("Ctrl+Backspace")
	}
	key(c, VKEscape)
	if c.Typing() || !c.Visible() {
		t.Fatal("Esc — назад")
	}
	// щелчок по строке результатов
	key(c, VKReturn)
	typeText(c, "смо")
	c.HoverAt(-1) // мышь ушла — выделение остаётся
	if r := c.Click("row", 0); r.Act == nil {
		t.Fatal("щелчок")
	} else {
		r.Act()
	}
	if src.picks[len(src.picks)-1] != "sound:smoke" {
		t.Fatal(src.picks)
	}
	// Alt+Tab: меню закрывается, клавиша — системе
	c.Open()
	key(c, VKReturn)
	if r := input(c, KeyInput{VK: VKTab, Alt: true}); r.Handled || !r.Close || c.Visible() {
		t.Fatal("Alt+Tab")
	}
	// без синтеза речи строки «Сказать» нет; таймаут при наборе — не меньше 45 с
	src.canSay, src.timeout = false, 5
	c.Open()
	key(c, VKReturn)
	typeText(c, "zzz")
	if v := c.View(1); len(v.Items) != 0 || v.Empty == "" {
		t.Fatalf("%+v", v)
	}
	if c.Expired(time.Now().Add(10 * time.Second)) {
		t.Fatal("закрылось, пока печатают")
	}
}

func TestFind(t *testing.T) {
	items := []Item{{ID: "a", Label: "Ёлка"}, {ID: "b", Label: "Большая ёлка"}, {ID: "c", Label: "Ель", Find: "дерево"}}
	if got := Find(items, "елк", 5); len(got) != 2 || got[0].ID != "a" {
		t.Fatalf("%+v", got)
	}
	if got := Find(items, "дер", 5); len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("%+v", got)
	}
	if got := Find(items, "ел", 1); len(got) != 1 {
		t.Fatal("limit")
	}
	if swapLayout("ghbdtn") != "привет" || swapLayout("руддщ") != "hello" {
		t.Fatal(swapLayout("ghbdtn"), swapLayout("руддщ"))
	}
}
