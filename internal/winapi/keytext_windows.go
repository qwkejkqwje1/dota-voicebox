package winapi

import (
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pToUnicodeEx       = user32.NewProc("ToUnicodeEx")
	pGetKeyboardLayout = user32.NewProc("GetKeyboardLayout")
	pGetKeyState       = user32.NewProc("GetKeyState")
)

// keyText — какой символ печатает клавиша в раскладке активного окна (игры): для строки поиска в меню
// поверх игры. Флаг 4 — не трогать состояние клавиатуры (мёртвые клавиши в игре не ломаются).
func keyText(vk, scan uint32, shift, altGr bool) string {
	fg, _, _ := pGetForegroundWindow.Call()
	tid, _, _ := pGetWindowThreadProcessId.Call(fg, 0)
	hkl, _, _ := pGetKeyboardLayout.Call(tid)
	var st [256]byte
	if shift {
		st[0x10], st[0xA0] = 0x80, 0x80
	}
	if altGr {
		st[0x11], st[0x12], st[0xA2], st[0xA5] = 0x80, 0x80, 0x80, 0x80
	}
	if ks, _, _ := pGetKeyState.Call(0x14); ks&1 != 0 { // Caps Lock
		st[0x14] = 0x01
	}
	var buf [8]uint16
	n, _, _ := pToUnicodeEx.Call(uintptr(vk), uintptr(scan), uintptr(unsafe.Pointer(&st[0])),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 4, hkl)
	if k := int(int32(n)); k > 0 && k <= len(buf) {
		out := []rune{}
		for _, r := range windows.UTF16ToString(buf[:k]) {
			if unicode.IsPrint(r) {
				out = append(out, r)
			}
		}
		return string(out)
	}
	return ""
}
