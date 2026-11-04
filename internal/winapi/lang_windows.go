package winapi

import (
	"syscall"
	"unsafe"
)

var (
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
	procLCIDToLocaleName         = kernel32.NewProc("LCIDToLocaleName")
)

// UILanguage — язык интерфейса Windows («ru-RU», «en-US»…); "" — не удалось узнать.
func UILanguage() string {
	id, _, _ := procGetUserDefaultUILanguage.Call()
	if id == 0 {
		return ""
	}
	buf := make([]uint16, 85)
	n, _, _ := procLCIDToLocaleName.Call(id&0xffff, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
