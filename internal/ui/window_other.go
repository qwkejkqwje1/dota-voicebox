//go:build !windows

package ui

import "log"

// Title — заголовок окна.
var Title = "VoiceBox"

func OpenWindow(url string) bool {
	log.Printf("Интерфейс: %s", url)
	return false
}

func FocusWindow() bool { return false }
func CloseWindow()      {}
