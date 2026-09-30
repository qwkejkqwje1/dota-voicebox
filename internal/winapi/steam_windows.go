//go:build windows

package winapi

import "golang.org/x/sys/windows/registry"

// SteamRoot читает путь Steam из реестра.
func SteamRoot() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		return `C:\Program Files (x86)\Steam`
	}
	defer k.Close()
	p, _, err := k.GetStringValue("SteamPath")
	if err != nil {
		return `C:\Program Files (x86)\Steam`
	}
	return p
}
