package overlay

import (
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
)

// «Читалка» поверх игры: строка набора, справа — выбранный голос. Пока текст не набран, строки — недавние
// фразы (Enter — прочитать ещё раз), с текстом — голоса персонажей (↑↓ ← → выбор). Enter — прочитать
// (слышат команда и вы — как в настройках), Ctrl+Enter — прочитать только себе.

const readColor = "#0ea5e9"

// StartReader открывает «Читалку» (меню должно быть открыто).
func (c *Controller) StartReader() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.visible {
		return
	}
	c.reader, c.vstart = true, 0
	c.syncVoiceLocked()
	c.startTypingLocked()
}

// syncVoiceLocked — выбранный голос из настроек приложения.
func (c *Controller) syncVoiceLocked() {
	c.voice = 0
	for i, v := range c.data.Voices {
		if v.ID == c.data.Voice {
			c.voice = i
		}
	}
}

// Reading — открыта ли «Читалка».
func (c *Controller) Reading() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.visible && c.typing && c.reader
}

func (c *Controller) curVoiceLocked() (Item, bool) {
	if c.voice >= 0 && c.voice < len(c.data.Voices) {
		return c.data.Voices[c.voice], true
	}
	return Item{}, false
}

// fitVoicesLocked — выбранный голос в пределах списка и в показанном окне строк.
func (c *Controller) fitVoicesLocked() {
	n := len(c.data.Voices)
	if n == 0 {
		c.voice, c.vstart = 0, 0
		return
	}
	c.voice = max(0, min(c.voice, n-1))
	if c.voice < c.vstart {
		c.vstart = c.voice
	}
	if c.voice >= c.vstart+searchRows {
		c.vstart = c.voice - searchRows + 1
	}
	c.vstart = max(0, min(c.vstart, n-searchRows))
}

// readerRowsLocked — строки «Читалки»: недавние фразы (пустая строка) или голоса.
func (c *Controller) readerRowsLocked() {
	var out []Item
	if strings.TrimSpace(string(c.query)) == "" {
		for _, p := range c.data.ReadHist {
			if len(out) == searchRows {
				break
			}
			out = append(out, Item{Kind: "readhist", ID: p, Emoji: "📝", Label: p, Color: readColor})
		}
		c.results = out
		switch {
		case len(out) == 0:
			c.hover = -1
		case c.hover < 0 || c.hover >= len(out):
			c.hover = 0
		}
		return
	}
	c.fitVoicesLocked()
	for i := c.vstart; i < min(c.vstart+searchRows, len(c.data.Voices)); i++ {
		v := c.data.Voices[i]
		v.Kind, v.Hint = "voice", v.Find
		if v.Color == "" {
			v.Color = readColor
		}
		out = append(out, v)
	}
	c.results = out
	c.hover = -1
	if len(out) > 0 {
		c.hover = c.voice - c.vstart
	}
}

// stepVoiceLocked — соседний голос по кругу.
func (c *Controller) stepVoiceLocked(d int) {
	n := len(c.data.Voices)
	if n == 0 {
		return
	}
	c.voice = ((c.voice+d)%n + n) % n
	c.fitVoicesLocked()
	c.readerRowsLocked()
}

// readerKeyLocked — клавиши «Читалки»; false — обычная обработка строки набора (буквы, Backspace, ↑↓ по фразам).
func (c *Controller) readerKeyLocked(k KeyInput) (Result, bool) {
	typed := strings.TrimSpace(string(c.query)) != ""
	switch k.VK {
	case VKEscape: // «Читалку» открыли отдельной клавишей — Esc закрывает меню целиком
		c.stopTypingLocked()
		c.visible = false
		return Result{Handled: true, Close: true}, true
	case VKReturn:
		return c.readActivateLocked(c.hover, k.Ctrl), true
	case VKLeft, VKRight, VKTab:
		d := 1
		if k.VK == VKLeft {
			d = -1
		}
		c.stepVoiceLocked(d)
		return Result{Handled: true, Redraw: true}, true
	case VKUp, VKDown:
		if !typed {
			return Result{}, false
		}
		d := 1
		if k.VK == VKUp {
			d = -1
		}
		c.stepVoiceLocked(d)
		return Result{Handled: true, Redraw: true}, true
	case VKPageUp, VKPageDown:
		d := searchRows
		if k.VK == VKPageUp {
			d = -d
		}
		c.voice = max(0, min(c.voice+d, len(c.data.Voices)-1))
		c.fitVoicesLocked()
		c.readerRowsLocked()
		return Result{Handled: true, Redraw: true}, true
	}
	return Result{}, false
}

// readActivateLocked — прочитать: набранный текст выбранным (или щёлкнутым) голосом либо недавнюю фразу.
// me — только себе (Ctrl+Enter).
func (c *Controller) readActivateLocked(i int, me bool) Result {
	q := strings.TrimSpace(string(c.query))
	text := q
	voice := ""
	if v, ok := c.curVoiceLocked(); ok {
		voice = v.ID
	}
	switch {
	case q != "":
		if i >= 0 && i < len(c.results) && c.results[i].Kind == "voice" {
			voice = c.results[i].ID
		}
	case i >= 0 && i < len(c.results):
		text = c.results[i].ID
	default:
		return Result{Handled: true}
	}
	kind := "read:" + voice
	if me {
		kind = "readme:" + voice
	}
	c.stopTypingLocked()
	c.visible = false
	src := c.Src
	return Result{Handled: true, Close: true, Act: func() { src.Pick(kind, text) }}
}

// readerViewLocked — как нарисовать «Читалку».
func (c *Controller) readerViewLocked(scale float64) MenuView {
	v := MenuView{Layout: LayoutList, Typing: true, Query: string(c.query), Hover: c.hover, Pages: 1, Scale: scale,
		Opacity: c.data.Opacity, Hint: i18n.T("Напишите, что прочитать")}
	if vo, ok := c.curVoiceLocked(); ok {
		v.Badge, v.BadgeEmoji = vo.Label, vo.Emoji
	}
	for i, it := range c.results {
		it.Key = ""
		if it.Kind == "readhist" {
			it.Hint = ""
			if i == c.hover {
				it.Hint = i18n.T("Enter — ещё раз")
			}
		}
		v.Items = append(v.Items, it)
	}
	typed := strings.TrimSpace(string(c.query)) != ""
	switch {
	case !c.data.CanSay:
		v.Empty = i18n.T("Голоса Windows недоступны — читать нечем")
	case len(v.Items) == 0 && !typed:
		v.Empty = i18n.T("Напишите — прочитаю голосом персонажа")
	}
	switch {
	case typed:
		v.Footer = i18n.T("↑↓ голос  ·  Enter прочитать  ·  Ctrl+Enter себе  ·  Esc")
	case len(v.Items) > 0:
		v.Footer = i18n.T("← → голос  ·  ↑↓ фраза  ·  Enter  ·  Esc")
	default:
		v.Footer = i18n.T("← → голос  ·  Esc закрыть")
	}
	return v
}
