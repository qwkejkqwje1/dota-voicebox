package app

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Управление звуками панели: добавить файл / фразу / встроенный звук, изменить, удалить, порядок, клавиши.

// Palette — цвета плиток по умолчанию.
var Palette = []string{"#8b5cf6", "#6366f1", "#3b82f6", "#06b6d4", "#10b981", "#84cc16", "#f59e0b", "#f97316", "#ef4444", "#ec4899"}

func colorFor(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return Palette[h.Sum32()%uint32(len(Palette))]
}

// Visible — показывается ли звук на панели.
func Visible(s *sounds.Sound) bool {
	return s != nil && (s.Source == "config" || s.Source == "file") && !s.Hidden
}

// DisplayLabel — название плитки: подпись или имя файла.
func DisplayLabel(s *sounds.Sound) string {
	if s.Label != "" {
		return s.Label
	}
	l := strings.NewReplacer("_", " ", "-", " ").Replace(s.ID)
	r := []rune(strings.TrimSpace(l))
	if len(r) == 0 {
		return s.ID
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// shown — плитка на панели: звуки модуля Dota спрятаны, пока модуль выключен.
func shown(cfg *config.Config, s *sounds.Sound) bool {
	return Visible(s) && (!s.Dota || cfg.Dota.Enabled)
}

// PadOrder — плитки панели по порядку: сначала из "pad", затем остальные видимые по названию.
func (a *App) PadOrder() []string {
	a.mu.Lock()
	cfg, lib := a.cfg, a.lib
	a.mu.Unlock()
	return padOrder(cfg, lib)
}

func padOrder(cfg *config.Config, lib *sounds.Library) []string {
	var out []string
	seen := map[string]bool{}
	for _, id := range cfg.Pad {
		if s := lib.Get(id); shown(cfg, s) && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	var rest []*sounds.Sound
	for _, id := range lib.IDs() {
		if s := lib.Get(id); shown(cfg, s) && !seen[id] {
			rest = append(rest, s)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		return strings.ToLower(DisplayLabel(rest[i])) < strings.ToLower(DisplayLabel(rest[j]))
	})
	for _, s := range rest {
		out = append(out, s.ID)
	}
	return out
}

var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
	'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'у': "u", 'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "", 'ы': "y", 'ь': "",
	'э': "e", 'ю': "yu", 'я': "ya",
}

// Slug — латинский идентификатор из названия: "Мид, ганк!" → "mid_gank".
func Slug(s string) string {
	var b strings.Builder
	under := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			under = false
		case translit[r] != "" || r == 'ъ' || r == 'ь':
			b.WriteString(translit[r])
			under = false
		default:
			if !under && b.Len() > 0 {
				b.WriteByte('_')
				under = true
			}
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "sound"
	}
	return out
}

func (a *App) uniqueID(cfg *config.Config, base string) string {
	a.mu.Lock()
	lib := a.lib
	a.mu.Unlock()
	taken := func(id string) bool {
		if _, ok := cfg.Sounds[id]; ok {
			return true
		}
		if lib != nil {
			if s := lib.Get(id); s != nil && s.Source != "builtin" {
				return true
			}
		}
		return false
	}
	id := base
	for n := 2; taken(id); n++ {
		id = fmt.Sprintf("%s_%d", base, n)
	}
	return id
}

func cutLabel(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 40 {
		s = string(r[:40]) + "…"
	}
	return s
}

// ImportSound сохраняет загруженный файл (WAV или MP3) в папку звуков и добавляет плитку.
func (a *App) ImportSound(name string, data []byte, ext string) (string, error) {
	ext = strings.ToLower(ext)
	var err error
	switch ext {
	case ".wav":
		_, err = sounds.DecodeWAV(data)
	case ".mp3":
		_, err = sounds.DecodeMP3(data)
	default:
		return "", fmt.Errorf("нужен WAV или MP3 (другие форматы интерфейс сам переводит в WAV)")
	}
	if err != nil {
		return "", fmt.Errorf("не удалось прочитать звук: %v", err)
	}
	name = strings.TrimSuffix(strings.TrimSpace(name), filepath.Ext(name))
	if name == "" {
		name = "Звук"
	}
	cfg := a.Config()
	id := a.uniqueID(cfg, Slug(name))
	dir := a.SoundsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	file := id + ext
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, file)); os.IsNotExist(err) {
			break
		}
		file = fmt.Sprintf("%s_%d%s", id, n, ext)
	}
	if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
		return "", err
	}
	soundsDir := cfg.SoundsDir
	if soundsDir == "" {
		soundsDir = "sounds"
	}
	def := sounds.Def{Files: []string{filepath.ToSlash(filepath.Join(soundsDir, file))}, Label: cutLabel(name),
		Emoji: "🔊", Color: colorFor(id), Mode: sounds.ModeToggle}
	return id, a.addDef(id, def)
}

// AddPhrase добавляет плитку-фразу (синтез речи).
func (a *App) AddPhrase(text, emoji string) (string, error) {
	return a.AddPhraseOpts(text, emoji, false)
}

// AddPhraseOpts — AddPhrase; dota = кнопка-оповещение модуля Dota 2.
func (a *App) AddPhraseOpts(text, emoji string, dota bool) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("пустая фраза")
	}
	if emoji == "" {
		emoji = "🗣️"
	}
	cfg := a.Config()
	id := a.uniqueID(cfg, "say_"+Slug(text))
	def := sounds.Def{Files: []string{"tts:" + text}, Label: cutLabel(text), Emoji: emoji,
		Color: colorFor(id), Mode: sounds.ModeInterrupt}
	if dota {
		id = a.uniqueID(cfg, "dota_"+Slug(text))
		def.Files = []string{"builtin:beep2", "tts:" + text}
		def.Seq, def.Dota, def.Category, def.Cooldown, def.Color = true, true, "Dota 2", 2, colorFor(id)
	}
	return id, a.addDef(id, def)
}

// AddBuiltin добавляет на панель встроенный звук.
func (a *App) AddBuiltin(name string) (string, error) {
	info, ok := builtinInfo(name)
	if !ok {
		return "", fmt.Errorf("нет встроенного звука %q", name)
	}
	cfg := a.Config()
	id := a.uniqueID(cfg, name)
	return id, a.addDef(id, sounds.Def{Files: []string{"builtin:" + name}, Label: info.Label, Emoji: info.Emoji,
		Color: colorFor(id), Mode: sounds.ModeToggle})
}

func builtinInfo(name string) (sounds.BuiltinInfo, bool) {
	for _, b := range sounds.BuiltinInfos() {
		if b.ID == name {
			return b, true
		}
	}
	return sounds.BuiltinInfo{}, false
}

func (a *App) addDef(id string, def sounds.Def) error {
	return a.UpdateConfig(func(c *config.Config) {
		c.Sounds[id] = def
		c.Pad = append(mergePad(padOrder(c, a.lib), c.Pad), id)
		if set := c.Set(c.ActiveSet); set != nil { // новый звук — и в открытый набор
			set.Sounds = append(set.Sounds, id)
		}
	})
}

// SoundEdit — изменения плитки из окна редактирования.
type SoundEdit struct {
	Label    string    `json:"label"`
	Emoji    string    `json:"emoji"`
	Color    string    `json:"color"`
	Bus      string    `json:"bus"`
	Volume   float64   `json:"volume"`
	Mode     string    `json:"mode"`
	Cooldown float64   `json:"cooldown"`
	Category string    `json:"category"`
	Text     *string   `json:"text,omitempty"`    // новый текст фразы
	Hotkey   *string   `json:"hotkey,omitempty"`  // "" — снять клавишу
	InMenu   *bool     `json:"in_menu,omitempty"` // показывать в быстром меню
	Sets     *[]string `json:"sets,omitempty"`    // в каких наборах звук
}

// UpdateSound применяет изменения плитки.
func (a *App) UpdateSound(id string, e SoundEdit) error {
	a.mu.Lock()
	lib := a.lib
	a.mu.Unlock()
	s := lib.Get(id)
	cfg := a.Config()
	d, ok := cfg.Sounds[id]
	if !ok {
		if s == nil {
			return fmt.Errorf("звук %q не найден", id)
		}
		d = sounds.Def{Files: s.Files}
	}
	if e.Hotkey != nil && *e.Hotkey != "" {
		if _, err := keys.Parse(*e.Hotkey); err != nil {
			return err
		}
	}
	d.Label, d.Emoji, d.Color, d.Category = cutLabel(e.Label), e.Emoji, e.Color, strings.TrimSpace(e.Category)
	d.Bus = sounds.ParseBus(e.Bus).String()
	if d.Bus == "both" {
		d.Bus = ""
	}
	d.Volume = clampF(e.Volume, 0, 2)
	switch e.Mode {
	case sounds.ModeToggle, sounds.ModeInterrupt, sounds.ModeOverlap, sounds.ModeIgnore:
		d.Mode = e.Mode
	}
	d.Cooldown = clampF(e.Cooldown, 0, 600)
	if e.Text != nil {
		t := strings.TrimSpace(*e.Text)
		if t == "" {
			return fmt.Errorf("пустая фраза")
		}
		for i, f := range d.Files {
			if strings.HasPrefix(f, "tts:") {
				d.Files[i] = "tts:" + t
			}
		}
	}
	return a.UpdateConfig(func(c *config.Config) {
		c.Sounds[id] = d
		if e.Hotkey != nil {
			bindHotkey(c, "sound:"+id, *e.Hotkey)
		}
		if e.InMenu != nil {
			setInMenu(c, a.lib, id, *e.InMenu)
		}
		if e.Sets != nil {
			setMembership(c, id, *e.Sets)
		}
	})
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// DeleteSound убирает плитку. Файлы, которые больше нигде не используются, переносятся в папку trash/.
func (a *App) DeleteSound(id string) error {
	a.mu.Lock()
	lib := a.lib
	a.mu.Unlock()
	cfg := a.Config()
	var files []string
	if d, ok := cfg.Sounds[id]; ok {
		files = d.Files
	} else if s := lib.Get(id); s != nil && s.Source == "file" {
		files = s.Files
	} else {
		return fmt.Errorf("звук %q не найден", id)
	}
	used := map[string]bool{}
	for oid, d := range cfg.Sounds {
		if oid != id {
			for _, f := range d.Files {
				used[f] = true
			}
		}
	}
	for _, f := range files {
		if used[f] || strings.Contains(f, ":") || filepath.IsAbs(f) {
			continue
		}
		src := filepath.Join(a.BaseDir, filepath.FromSlash(f))
		trash := filepath.Join(a.BaseDir, "trash")
		os.MkdirAll(trash, 0o755)
		if err := os.Rename(src, filepath.Join(trash, filepath.Base(src))); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return a.UpdateConfig(func(c *config.Config) {
		delete(c.Sounds, id)
		c.Pad = remove(c.Pad, id)
		c.QuickMenu.Items = remove(c.QuickMenu.Items, id)
		setMembership(c, id, nil)
		if c.Background.ID == "sound:"+id {
			c.Background.ID = ""
		}
		for k, act := range c.Hotkeys {
			if a, _, _ := strings.Cut(act, "@"); a == "sound:"+id {
				delete(c.Hotkeys, k)
			}
		}
	})
}

func remove(list []string, id string) []string {
	out := list[:0:0]
	for _, x := range list {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// SetPad сохраняет порядок плиток. Плитки, которых нет в списке (например, спрятанные звуки Dota), остаются в конце.
func (a *App) SetPad(order []string) error {
	return a.saveQuiet(func(c *config.Config) { c.Pad = mergePad(order, c.Pad) })
}

// mergePad — новый порядок плиток плюс те из старого, которых в нём нет.
func mergePad(order, old []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, list := range [][]string{order, old} {
		for _, id := range list {
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// BindHotkey назначает клавишу действию ("" — снять). Клавиша, занятая другим действием, переходит к этому.
func (a *App) BindHotkey(action, key string) error {
	if key != "" {
		if _, err := keys.Parse(key); err != nil {
			return err
		}
	}
	return a.UpdateConfig(func(c *config.Config) { bindHotkey(c, action, key) })
}

func bindHotkey(c *config.Config, action, key string) {
	for k, act := range c.Hotkeys {
		if act == action {
			delete(c.Hotkeys, k)
		}
	}
	if key == "" {
		return
	}
	nc, err := keys.Parse(key)
	if err != nil {
		return
	}
	for k := range c.Hotkeys {
		if o, err := keys.Parse(k); err == nil && o.VK == nc.VK && o.Mods == nc.Mods {
			delete(c.Hotkeys, k)
		}
	}
	c.Hotkeys[key] = action
}

// HotkeyMap — действие → клавиша (для подсказок на плитках).
func (a *App) HotkeyMap() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]string{}
	ks := make([]string, 0, len(a.cfg.Hotkeys))
	for k := range a.cfg.Hotkeys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		act := a.cfg.Hotkeys[k]
		if _, ok := out[act]; !ok {
			out[act] = k
		}
	}
	return out
}

// SetQuickMenu сохраняет настройки быстрого меню.
func (a *App) SetQuickMenu(q config.QuickMenu) error {
	return a.saveQuiet(func(c *config.Config) { c.QuickMenu = q })
}

// MoveQuickMenu — меню перетащили мышью поверх игры: дальше оно появляется на этом месте.
func (a *App) MoveQuickMenu(x, y float64) error {
	return a.saveQuiet(func(c *config.Config) {
		c.QuickMenu.Position, c.QuickMenu.X, c.QuickMenu.Y = "custom", x, y
	})
}

func setInMenu(c *config.Config, lib *sounds.Library, id string, on bool) {
	items := c.QuickMenu.Items
	if len(items) == 0 { // режим «авто» → явный список
		items = userMenuSounds(c, lib)
	}
	has := false
	for _, x := range items {
		has = has || x == id
	}
	switch {
	case on && !has:
		items = append(items, id)
	case !on && has:
		items = remove(items, id)
	}
	c.QuickMenu.Items = items
}

// SetDota сохраняет настройки модуля Dota (перезагрузка: напоминания, GSI, звуки).
func (a *App) SetDota(d config.Dota) error {
	return a.UpdateConfig(func(c *config.Config) { c.Dota = d })
}
