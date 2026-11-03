package config

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Default — конфиг по умолчанию (новая установка).
func Default() *Config {
	var c Config
	if err := json.Unmarshal(DefaultJSON, &c); err != nil {
		panic(fmt.Sprintf("встроенный конфиг: %v", err))
	}
	c.Fix()
	return &c
}

// migrate обновляет старый конфиг до текущего формата (настройки пользователя сохраняются).
func migrate(c *Config, probe map[string]json.RawMessage) {
	if c.Version < 2 {
		// v1 → v2: появились пульт (mic, sources) и автообновление — берём значения по умолчанию,
		// они уже подставлены из встроенного конфига.
		if _, ok := probe["update"]; !ok {
			c.Update.AutoCheck, c.Update.AutoInstall = true, true
		}
	}
	if c.Version < 3 {
		migrateV3(c, probe)
	}
	if c.Version < 4 {
		migrateV4(c)
	}
	c.Version = Version
}

// SupportRunes — напоминание саппортам за 40 секунд до рун силы на 6:00 (слышит команда).
var SupportRunes = Reminder{ID: "support_runes", Label: "Саппорты к рунам", Enabled: true, At: []int{360}, Before: 40, To: "both",
	Say: "Саппорты, руны силы {время}. Перетянитесь и подконтрольте руны"}

// v3 → v4: напоминания с ограничением по времени — руны силы важны на 6:00, лотосы — до 9:00;
// новое напоминание саппортам. Меняем только то, что пользователь не трогал.
func migrateV4(c *Config) {
	rs := c.Dota.Reminders
	has := false
	at := -1
	for i := range rs {
		r := &rs[i]
		plain := len(r.At) == 0 && r.Until == 0 && len(r.Skip) == 0
		switch r.ID {
		case "power":
			if plain && r.Start == 360 && r.Every == 120 {
				r.Start, r.Every, r.At = 0, 0, []int{360}
			}
			at = i
		case "lotus":
			if plain && r.Start == 180 && r.Every == 180 {
				r.Until = 540
			}
		case SupportRunes.ID:
			has = true
		}
	}
	if !has && len(rs) > 0 {
		sr := SupportRunes
		sr.At = []int{360}
		i := at + 1
		if at < 0 {
			i = len(rs)
		}
		rs = append(rs[:i], append([]Reminder{sr}, rs[i:]...)...)
		c.Dota.Reminders = rs
	}
}

// Звуки v2, которые были нужны только таймерам и событиям, — теперь служебные звуки модуля Dota.
var v2ServiceSounds = map[string]string{
	"wisdom_rune": "builtin:wisdom_rune", "mid_rune_6": "builtin:mid_rune_6", "power_rune": "builtin:power_rune",
	"bounty_rune": "builtin:bounty_rune", "rosh": "builtin:rosh", "low_hp": "builtin:danger",
}

// v2 → v3: саундпад с плитками, быстрое меню, модуль Dota с голосовыми напоминаниями.
func migrateV3(c *Config, probe map[string]json.RawMessage) {
	if c.Sounds == nil {
		c.Sounds = map[string]sounds.Def{}
	}
	if c.Hotkeys == nil {
		c.Hotkeys = map[string]string{}
	}
	if _, ok := probe["pad"]; !ok {
		c.Pad = nil // не брать панель из конфига по умолчанию — соберём из звуков пользователя
	}
	var userIDs []string
	for id, d := range c.Sounds {
		if d.Emoji == "" {
			d.Emoji, d.Label = SplitEmoji(d.Label)
		}
		if f, ok := v2ServiceSounds[id]; ok && len(d.Files) == 1 && d.Files[0] == f {
			d.Dota, d.Hidden = true, true
		}
		if id == "siren" && len(d.Files) == 1 && d.Files[0] == "builtin:siren" {
			d.Dota = true
		}
		if d.Dota && !d.Hidden && d.Category == "" {
			d.Category = "Dota 2"
		}
		c.Sounds[id] = d
		if !d.Hidden {
			userIDs = append(userIDs, id)
		}
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return strings.ToLower(labelOf(c, userIDs[i])) < strings.ToLower(labelOf(c, userIDs[j]))
	})

	// таймеры → голосовые напоминания
	if len(c.Timers) > 0 {
		var rs []Reminder
		for _, t := range c.Timers {
			r := Reminder{ID: t.ID, Label: t.Label, Enabled: !t.Disabled, At: t.At, Start: t.Start, Every: t.Every,
				Until: t.Until, Skip: t.Skip, Before: t.WarnBefore}
			if r.Label == "" {
				r.Label = t.ID
			}
			if snd, _, _ := strings.Cut(t.Sound, "@"); snd != "" && snd != "stack" {
				if _, std := v2ServiceSounds[snd]; !std {
					r.Sound = snd // свой звук пользователя оставляем сигналом
				}
			}
			if t.ID == "stack" {
				r.Say = "Стак"
			}
			rs = append(rs, r)
		}
		c.Dota.Reminders = rs
		c.Timers = nil
	}
	c.Dota.Enabled = true // старые версии были только для Dota
	c.Dota.Rosh = true

	// новые звуки: стартовый набор и кнопки-оповещения Dota (скрипт gank_mid → кнопка)
	gank := c.Scripts["gank_mid"]
	if _, ok := probe["scripts"]; !ok {
		gank = true // в v2 скрипт был включён по умолчанию
	}
	if gank {
		if c.Scripts == nil {
			c.Scripts = map[string]bool{}
		}
		c.Scripts["gank_mid"] = false
	}
	def := Default()
	var added []string
	for _, id := range def.Pad {
		if _, ok := c.Sounds[id]; ok {
			continue
		}
		c.Sounds[id] = def.Sounds[id]
		added = append(added, id)
	}
	if len(c.Pad) == 0 {
		c.Pad = append(userIDs, added...)
	}
	// горячие клавиши по умолчанию — только если клавиша свободна
	for k, act := range def.Hotkeys {
		switch {
		case strings.HasPrefix(act, "sound:dota_") && !gank:
			continue // кнопки ганка были в скрипте; если он был выключен — не навязываем клавиши
		case act == "ui", HasAction(c.Hotkeys, act):
			continue
		}
		if FreeKey(c.Hotkeys, k) {
			c.Hotkeys[k] = act
		}
	}
	c.UI.KeepRunningOnClose = true // окно закрывается в трей: быстрое меню и клавиши продолжают работать
}

func labelOf(c *Config, id string) string {
	if l := c.Sounds[id].Label; l != "" {
		return l
	}
	return id
}

// SplitEmoji отделяет значок в начале подписи: "🚨 Мид ганкают" → "🚨", "Мид ганкают".
func SplitEmoji(label string) (emoji, rest string) {
	label = strings.TrimSpace(label)
	i := strings.IndexByte(label, ' ')
	if i <= 0 {
		return "", label
	}
	head := label[:i]
	for _, r := range head {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r < 0x2000 {
			return "", label
		}
	}
	return head, strings.TrimSpace(label[i+1:])
}

// HasAction — назначено ли действие на какую-нибудь клавишу.
func HasAction(hk map[string]string, act string) bool {
	for _, a := range hk {
		if a == act {
			return true
		}
	}
	return false
}

// FreeKey — не занята ли клавиша (сравнение по разобранному сочетанию, а не по строке).
func FreeKey(hk map[string]string, key string) bool {
	c, err := keys.Parse(key)
	if err != nil {
		return false
	}
	for k := range hk {
		if o, err := keys.Parse(k); err == nil && o.VK == c.VK && o.Mods == c.Mods {
			return false
		}
	}
	return true
}

// Fix приводит значения в порядок после загрузки (пустые поля, дубликаты).
func (c *Config) Fix() {
	if c.Hotkeys == nil {
		c.Hotkeys = map[string]string{}
	}
	if c.Sounds == nil {
		c.Sounds = map[string]sounds.Def{}
	}
	q := &c.QuickMenu
	if q.Scale <= 0 {
		q.Scale = 1
	}
	q.Scale = clamp(q.Scale, 0.7, 2)
	switch q.Position {
	case "center", "top", "bottom", "left", "right", "top-left", "top-right", "bottom-left", "bottom-right", "cursor", "custom":
	default:
		q.Position = "center"
	}
	switch q.Layout {
	case "list", "strip", "grid":
	default:
		q.Layout = "list"
	}
	switch q.Monitor {
	case "game", "cursor", "primary":
	default:
		q.Monitor = "game"
	}
	q.X, q.Y = clamp(q.X, 0, 1), clamp(q.Y, 0, 1)
	if q.Opacity <= 0 {
		q.Opacity = 0.95
	}
	q.Opacity = clamp(q.Opacity, 0.3, 1)
	if q.TimeoutSec < 0 {
		q.TimeoutSec = 0
	}
	q.Items = dedupe(q.Items)
	q.Effects = dedupe(q.Effects)
	c.fixWheel()
	c.fixRemote()
	c.fixVoice()
	c.fixSets()
	c.Pad = dedupe(c.Pad)
	d := &c.Dota
	if d.Volume <= 0 {
		d.Volume = 1
	}
	switch d.VoiceTo {
	case "monitor", "both", "voice":
	default:
		d.VoiceTo = "monitor"
	}
	seen := map[string]bool{}
	for i := range d.Reminders {
		r := &d.Reminders[i]
		if r.ID == "" || seen[r.ID] {
			for n := i + 1; ; n++ {
				id := fmt.Sprintf("r%d", n)
				if !seen[id] {
					r.ID = id
					break
				}
			}
		}
		seen[r.ID] = true
		if r.Before < 0 {
			r.Before = 0
		}
	}
}

// EQBands — полос в своём тембре.
const EQBands = 5

func (c *Config) fixVoice() {
	m := &c.Mic
	if m.DenoiseLevel <= 0 || m.DenoiseLevel > 1 {
		m.DenoiseLevel = 0.92
	}
	if len(m.EQ) > 0 || m.Timbre == "custom" {
		eq := make([]float64, EQBands)
		for i := range eq {
			if i < len(m.EQ) {
				eq[i] = clamp(m.EQ[i], -12, 12)
			}
		}
		m.EQ = eq
	}
	m.Smooth = clamp(m.Smooth, 0, 1)
	b := &c.Background
	b.Volume = clamp(b.Volume, 0, 1)
}

func (c *Config) fixSets() {
	seen := map[string]bool{}
	out := c.Sets[:0]
	for _, s := range c.Sets {
		s.Name = strings.TrimSpace(s.Name)
		if s.ID == "" || seen[s.ID] || s.Name == "" {
			continue
		}
		seen[s.ID] = true
		s.Sounds = dedupe(s.Sounds)
		var games []string
		for _, g := range s.Games {
			if g = strings.ToLower(strings.TrimSpace(g)); g != "" {
				games = append(games, g)
			}
		}
		s.Games = dedupe(games)
		out = append(out, s)
	}
	c.Sets = out
	if !seen[c.ActiveSet] {
		c.ActiveSet = ""
	}
}

// Set — набор по id (nil — нет такого или "" — все звуки).
func (c *Config) Set(id string) *SoundSet {
	for i := range c.Sets {
		if c.Sets[i].ID == id {
			return &c.Sets[i]
		}
	}
	return nil
}

func (c *Config) fixWheel() {
	w := &c.Wheel
	if w.Scale <= 0 {
		w.Scale = 1
	}
	w.Scale = clamp(w.Scale, 0.7, 2)
	if w.Opacity <= 0 {
		w.Opacity = 0.92
	}
	w.Opacity = clamp(w.Opacity, 0.3, 1)
	if w.Center != "screen" {
		w.Center = "cursor"
	}
	if len(w.Items) > WheelSlots {
		w.Items = w.Items[:WheelSlots]
	}
	for i, s := range w.Items {
		w.Items[i] = strings.TrimSpace(s)
	}
	if len(w.Items) == 0 {
		w.Items = nil // первые 8 звуков с панели
		return
	}
	for len(w.Items) < WheelSlots { // свой набор: всегда ровно 8 мест, пустые сектора остаются на своих местах
		w.Items = append(w.Items, "")
	}
}

// RemoteKeyChars — алфавит ключа пульта: без похожих символов (0/o, 1/l/i), чтобы адрес легко перепечатать.
const RemoteKeyChars = "abcdefghjkmnpqrstuvwxyz23456789"

// NewRemoteKey — новый случайный ключ для адреса пульта.
func NewRemoteKey() string {
	const n = len(RemoteKeyChars)
	out := make([]byte, 0, 12)
	buf := make([]byte, 32)
	for len(out) < cap(out) {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		for _, v := range buf {
			if int(v) < 256/n*n && len(out) < cap(out) { // без перекоса: лишние значения отбрасываем
				out = append(out, RemoteKeyChars[int(v)%n])
			}
		}
	}
	return string(out)
}

// ValidRemoteKey — ключ из допустимых символов и не короче 8.
func ValidRemoteKey(k string) bool {
	if len(k) < 8 || len(k) > 64 {
		return false
	}
	for _, ch := range k {
		if !strings.ContainsRune(RemoteKeyChars, ch) {
			return false
		}
	}
	return true
}

func (c *Config) fixRemote() {
	r := &c.Remote
	if r.Port != 0 && (r.Port < 1024 || r.Port > 65535) {
		r.Port = 0
	}
	if r.Sounds != "menu" {
		r.Sounds = "pad"
	}
	r.Key = strings.ToLower(strings.TrimSpace(r.Key))
	if r.Key != "" && !ValidRemoteKey(r.Key) {
		r.Key = ""
	}
	if r.Enabled && r.Key == "" {
		r.Key = NewRemoteKey()
	}
}

// RemotePort — порт пульта (0 в настройках — порт интерфейса + 1).
func (c *Config) RemotePort() int {
	if c.Remote.Port != 0 {
		return c.Remote.Port
	}
	p := c.UI.Port
	if p == 0 {
		p = 3001
	}
	return p + 1
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func dedupe(in []string) []string {
	if in == nil {
		return nil
	}
	seen := map[string]bool{}
	out := in[:0:0]
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
