//go:build !windows

package ui

import "log"

func OpenWindow(url string) bool {
	log.Printf("Интерфейс: %s", url)
	return false
}
