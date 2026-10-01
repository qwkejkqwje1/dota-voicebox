//go:build windows

package winapi

import (
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// SteamRoot читает путь Steam из реестра.
func SteamRoot() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if p, _, err := k.GetStringValue("SteamPath"); err == nil && p != "" {
			return filepath.Clean(p)
		}
	}
	return `C:\Program Files (x86)\Steam`
}
