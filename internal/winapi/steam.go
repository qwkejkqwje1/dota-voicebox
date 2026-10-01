package winapi

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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
	if steamRoot == "" {
		return "", errors.New("Steam не найден")
	}
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

func GSIPath(dotaDir string) string {
	return filepath.Join(dotaDir, "game", "dota", "cfg", "gamestate_integration", "gamestate_integration_voicebox.cfg")
}

// InstallGSI кладёт cfg в game/dota/cfg/gamestate_integration.
func InstallGSI(dotaDir, uri, token string) (string, error) {
	p := GSIPath(dotaDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, os.WriteFile(p, []byte(GSIConfig(uri, token)), 0o644)
}

// GSIInstalled — установлен ли cfg с нужным адресом и токеном.
func GSIInstalled(dotaDir, uri, token string) bool {
	b, err := os.ReadFile(GSIPath(dotaDir))
	return err == nil && string(b) == GSIConfig(uri, token)
}

// ---------- клавиша голосового чата из биндов Dota 2 ----------

var voiceBindRe = regexp.MustCompile(`"([^"]+)"\s+"\+voicerecord"`)

// Source 2 имена клавиш → наши.
var sourceKeys = map[string]string{
	"SPACE": "Space", "TAB": "Tab", "CAPSLOCK": "CapsLock", "ENTER": "Enter", "ESCAPE": "Esc",
	"BACKSPACE": "Backspace", "INS": "Insert", "DEL": "Delete", "HOME": "Home", "END": "End",
	"PGUP": "PageUp", "PGDN": "PageDown", "UPARROW": "Up", "DOWNARROW": "Down",
	"LEFTARROW": "Left", "RIGHTARROW": "Right", "SHIFT": "LShift", "RSHIFT": "RShift",
	"CTRL": "LCtrl", "RCTRL": "RCtrl", "ALT": "LAlt", "RALT": "RAlt", "`": "Tilde",
	"MOUSE3": "Mouse3", "MOUSE4": "Mouse4", "MOUSE5": "Mouse5", "PAUSE": "Pause", "SCROLLLOCK": "ScrollLock",
	"KP_INS": "Num0", "KP_0": "Num0", "KP_END": "Num1", "KP_1": "Num1", "KP_DOWNARROW": "Num2", "KP_2": "Num2",
	"KP_PGDN": "Num3", "KP_3": "Num3", "KP_LEFTARROW": "Num4", "KP_4": "Num4", "KP_5": "Num5",
	"KP_RIGHTARROW": "Num6", "KP_6": "Num6", "KP_HOME": "Num7", "KP_7": "Num7", "KP_UPARROW": "Num8", "KP_8": "Num8",
	"KP_PGUP": "Num9", "KP_9": "Num9", "KP_PLUS": "NumAdd", "KP_MINUS": "NumSub", "KP_MULTIPLY": "NumMul",
	"KP_SLASH": "NumDiv", "KP_DEL": "NumDot",
}

// SourceKeyName переводит имя клавиши из конфига Dota в формат VoiceBox.
func SourceKeyName(k string) (string, bool) {
	up := strings.ToUpper(k)
	if v, ok := sourceKeys[up]; ok {
		return v, true
	}
	if len(up) == 1 && ((up[0] >= 'A' && up[0] <= 'Z') || (up[0] >= '0' && up[0] <= '9')) {
		return up, true
	}
	if regexp.MustCompile(`^F([1-9]|1[0-9]|2[0-4])$`).MatchString(up) {
		return up, true
	}
	return "", false
}

// FindVoiceKey ищет клавишу «+voicerecord» (голосовой чат команды) в биндах Dota 2.
// Берётся самый свежий файл биндов из userdata; иначе — стандартные бинды игры.
func FindVoiceKey(steamRoot, dotaDir string) (key, file string, err error) {
	type cand struct {
		key, file string
		mt        time.Time
	}
	var cands []cand
	scan := func(pattern string) {
		files, _ := filepath.Glob(pattern)
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			m := voiceBindRe.FindStringSubmatch(string(b))
			if m == nil {
				continue
			}
			st, _ := os.Stat(f)
			cands = append(cands, cand{m[1], f, st.ModTime()})
		}
	}
	if steamRoot != "" {
		for _, sub := range []string{"remote", "local"} {
			scan(filepath.Join(steamRoot, "userdata", "*", "570", sub, "cfg", "*.vcfg"))
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mt.After(cands[j].mt) })
	if len(cands) == 0 && dotaDir != "" {
		scan(filepath.Join(dotaDir, "game", "dota", "cfg", "*.vcfg"))
	}
	if len(cands) == 0 {
		return "", "", errors.New("бинд голосового чата не найден в конфигах Dota 2")
	}
	name, ok := SourceKeyName(cands[0].key)
	if !ok {
		return "", cands[0].file, fmt.Errorf("клавиша %q пока не поддерживается — задайте вручную", cands[0].key)
	}
	return name, cands[0].file, nil
}

// ---------- параметр запуска -gamestateintegration ----------

const GSILaunchOption = "-gamestateintegration"

func localConfigs(steamRoot string) []string {
	files, _ := filepath.Glob(filepath.Join(steamRoot, "userdata", "*", "config", "localconfig.vdf"))
	return files
}

func dotaApp(root *KV) *KV {
	store := root.Child("UserLocalConfigStore")
	if store == nil {
		return nil
	}
	steam := store.Ensure("Software").Ensure("Valve").Ensure("Steam")
	apps := steam.Child("apps")
	if apps == nil {
		apps = steam.Ensure("apps")
	}
	return apps.Ensure("570")
}

// HasLaunchOption — есть ли -gamestateintegration у Dota 2 во всех профилях Steam.
func HasLaunchOption(steamRoot string) (bool, error) {
	files := localConfigs(steamRoot)
	if len(files) == 0 {
		return false, errors.New("профили Steam не найдены")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return false, err
		}
		root, err := ParseVDF(string(b))
		if err != nil {
			return false, err
		}
		app := dotaApp(root)
		if app == nil {
			continue
		}
		if c := app.Child("LaunchOptions"); c == nil || !strings.Contains(c.Value, GSILaunchOption) {
			return false, nil
		}
	}
	return true, nil
}

// AddLaunchOption дописывает -gamestateintegration (Steam должен быть закрыт,
// иначе он перезапишет файл). Делает резервную копию localconfig.vdf.bak.
func AddLaunchOption(steamRoot string) (int, error) {
	n := 0
	for _, f := range localConfigs(steamRoot) {
		b, err := os.ReadFile(f)
		if err != nil {
			return n, err
		}
		root, err := ParseVDF(string(b))
		if err != nil {
			return n, fmt.Errorf("%s: %w", f, err)
		}
		app := dotaApp(root)
		if app == nil {
			continue
		}
		cur := ""
		if c := app.Child("LaunchOptions"); c != nil {
			cur = c.Value
		}
		if strings.Contains(cur, GSILaunchOption) {
			continue
		}
		app.Set("LaunchOptions", strings.TrimSpace(cur+" "+GSILaunchOption))
		os.WriteFile(f+".bak", b, 0o644)
		if err := os.WriteFile(f, []byte(root.String()), 0o644); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ---------- VB-Audio Virtual Cable ----------

var cableZipRe = regexp.MustCompile(`https?://download\.vb-audio\.com/Download_CABLE/VBCABLE_Driver_Pack\d+\.zip`)

const cableFallback = "https://download.vb-audio.com/Download_CABLE/VBCABLE_Driver_Pack45.zip"

// DownloadVBCable скачивает и распаковывает установщик. Возвращает путь к VBCABLE_Setup_x64.exe.
func DownloadVBCable(dir string) (string, error) {
	cl := &http.Client{Timeout: 60 * time.Second}
	url := cableFallback
	if r, err := cl.Get("https://vb-audio.com/Cable/"); err == nil {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		r.Body.Close()
		if m := cableZipRe.FindString(string(b)); m != "" {
			url = m
		}
	}
	r, err := cl.Get(url)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return "", fmt.Errorf("скачивание VB-Cable: HTTP %d", r.StatusCode)
	}
	os.MkdirAll(dir, 0o755)
	zipPath := filepath.Join(dir, "vbcable.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r.Body); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	setup := ""
	for _, zf := range zr.File {
		name := filepath.Base(zf.Name)
		if zf.FileInfo().IsDir() || strings.Contains(zf.Name, "..") {
			continue
		}
		dst := filepath.Join(dir, name)
		rc, err := zf.Open()
		if err != nil {
			return "", err
		}
		w, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return "", err
		}
		io.Copy(w, rc)
		w.Close()
		rc.Close()
		if strings.EqualFold(name, "VBCABLE_Setup_x64.exe") {
			setup = dst
		}
	}
	if setup == "" {
		return "", errors.New("в архиве нет VBCABLE_Setup_x64.exe")
	}
	return setup, nil
}
