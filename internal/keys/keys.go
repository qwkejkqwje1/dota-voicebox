// Package keys парсит строки горячих клавиш ("Ctrl+Alt+Num1") в коды Windows (VK).
package keys

import (
	"fmt"
	"strings"
)

// Модификаторы в формате RegisterHotKey.
const (
	ModAlt      = 0x0001
	ModCtrl     = 0x0002
	ModShift    = 0x0004
	ModWin      = 0x0008
	ModNoRepeat = 0x4000
)

// Combo — клавиша + модификаторы.
type Combo struct {
	Mods uint32
	VK   uint32
	Text string
}

var vk = map[string]uint32{
	"BACKSPACE": 0x08, "TAB": 0x09, "ENTER": 0x0D, "PAUSE": 0x13, "CAPSLOCK": 0x14,
	"ESC": 0x1B, "SPACE": 0x20, "PAGEUP": 0x21, "PAGEDOWN": 0x22, "END": 0x23, "HOME": 0x24,
	"LEFT": 0x25, "UP": 0x26, "RIGHT": 0x27, "DOWN": 0x28, "INSERT": 0x2D, "DELETE": 0x2E,
	"NUM0": 0x60, "NUM1": 0x61, "NUM2": 0x62, "NUM3": 0x63, "NUM4": 0x64,
	"NUM5": 0x65, "NUM6": 0x66, "NUM7": 0x67, "NUM8": 0x68, "NUM9": 0x69,
	"NUMMUL": 0x6A, "NUMADD": 0x6B, "NUMSUB": 0x6D, "NUMDOT": 0x6E, "NUMDIV": 0x6F,
	"SCROLLLOCK": 0x91, "TILDE": 0xC0, "`": 0xC0,
	"MOUSE3": 0x04, "MOUSE4": 0x05, "MOUSE5": 0x06, // XBUTTON1/2 — годятся только как PTT-клавиша для опроса
	"LSHIFT": 0xA0, "RSHIFT": 0xA1, "LCTRL": 0xA2, "RCTRL": 0xA3, "LALT": 0xA4, "RALT": 0xA5,
}

func init() {
	for c := 'A'; c <= 'Z'; c++ {
		vk[string(c)] = uint32(c)
	}
	for c := '0'; c <= '9'; c++ {
		vk[string(c)] = uint32(c)
	}
	for i := 1; i <= 24; i++ {
		vk[fmt.Sprintf("F%d", i)] = uint32(0x6F + i)
	}
}

// Parse разбирает "Ctrl+Shift+F9", "Num1", "V".
func Parse(s string) (Combo, error) {
	c := Combo{Text: s}
	parts := strings.Split(strings.ReplaceAll(s, " ", ""), "+")
	for i, p := range parts {
		up := strings.ToUpper(p)
		last := i == len(parts)-1
		if !last {
			switch up {
			case "CTRL", "CONTROL":
				c.Mods |= ModCtrl
			case "ALT":
				c.Mods |= ModAlt
			case "SHIFT":
				c.Mods |= ModShift
			case "WIN":
				c.Mods |= ModWin
			default:
				return c, fmt.Errorf("неизвестный модификатор %q в %q", p, s)
			}
			continue
		}
		code, ok := vk[up]
		if !ok {
			return c, fmt.Errorf("неизвестная клавиша %q в %q", p, s)
		}
		c.VK = code
	}
	if c.VK == 0 {
		return c, fmt.Errorf("пустая клавиша %q", s)
	}
	return c, nil
}

// VK возвращает код клавиши по имени ("Num1", "F9", "Mouse4").
func VK(name string) (uint32, bool) {
	v, ok := vk[strings.ToUpper(name)]
	return v, ok
}

// Name — обратное преобразование (для журнала и интерфейса).
func Name(code uint32) string {
	best := ""
	for k, v := range vk {
		if v == code && (best == "" || len(k) > len(best) || (len(k) == len(best) && k < best)) {
			best = k
		}
	}
	return best
}
