//go:build !windows

package winapi

import (
	"os"
	"strings"
)

// UILanguage — язык системы из LANG («en_US.UTF-8» → «en-US»); "" — не задан.
func UILanguage() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			v, _, _ = strings.Cut(v, ".")
			return strings.ReplaceAll(v, "_", "-")
		}
	}
	return ""
}
