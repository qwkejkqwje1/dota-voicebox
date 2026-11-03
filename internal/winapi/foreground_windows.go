package winapi

import "unsafe"

// ForegroundApp — программа активного окна ("dota2.exe"); "" — не удалось узнать или это сам VoiceBox.
func ForegroundApp() string {
	fg, _, _ := pGetForegroundWindow.Call()
	if fg == 0 {
		return ""
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(fg, uintptr(unsafe.Pointer(&pid)))
	if me, _, _ := pGetCurrentProcessId.Call(); pid == 0 || pid == uint32(me) {
		return ""
	}
	return procName(pid)
}
