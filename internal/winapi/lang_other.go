//go:build !windows

package winapi

import (
	"os"
	"strings"
)

// UILanguage — язык системы из LANG («en_US.UTF-8» → «en-US»); "" — не задан (и «C», «C.UTF-8», «POSIX»).
func UILanguage() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v, _, _ := strings.Cut(os.Getenv(k), ".")
		if v != "" && v != "C" && v != "POSIX" {
			return strings.ReplaceAll(v, "_", "-")
		}
	}
	return ""
}
