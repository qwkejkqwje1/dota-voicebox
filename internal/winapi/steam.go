package winapi

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GSIConfig — содержимое файла gamestate_integration_voicebox.cfg.
func GSIConfig(uri, token string) string {
	return `"Dota VoiceBox"
{
  "uri"       "` + uri + `"
  "timeout"   "5.0"
  "buffer"    "0.0"
  "throttle"  "0.1"
  "heartbeat" "30.0"
  "auth"
  {
    "token" "` + token + `"
  }
  "data"
  {
    "provider" "1"
    "map"      "1"
    "player"   "1"
    "hero"     "1"
  }
}
`
}

var libPathRe = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// FindDota ищет папку "dota 2 beta" во всех библиотеках Steam.
func FindDota(steamRoot string) (string, error) {
	roots := []string{steamRoot}
	if b, err := os.ReadFile(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, m := range libPathRe.FindAllStringSubmatch(string(b), -1) {
			roots = append(roots, strings.ReplaceAll(m[1], `\\`, `\`))
		}
	}
	for _, r := range roots {
		p := filepath.Join(r, "steamapps", "common", "dota 2 beta")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("Dota 2 не найдена в библиотеках Steam")
}

// InstallGSI кладёт cfg в game/dota/cfg/gamestate_integration.
func InstallGSI(dotaDir, uri, token string) (string, error) {
	dir := filepath.Join(dotaDir, "game", "dota", "cfg", "gamestate_integration")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "gamestate_integration_voicebox.cfg")
	return p, os.WriteFile(p, []byte(GSIConfig(uri, token)), 0o644)
}
