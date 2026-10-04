// Package config — загрузка config.json (с комментариями не работает: чистый JSON).
package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/timers"
)

//go:embed default.json
var DefaultJSON []byte

// Version — текущая версия формата конфига.
const Version = 2

type Config struct {
	Version int `json:"version"`
	Devices struct {
		Mic      string `json:"mic"`
		VoiceOut string `json:"voice_out"`
		Monitor  string `json:"monitor"`
	} `json:"devices"`
	MicGain       float64 `json:"mic_gain"`
	Ducking       float64 `json:"ducking"`
	FxOnSounds    bool    `json:"fx_on_sounds"`
	MonitorVolume float64 `json:"monitor_volume"`
	SfxVolume     float64 `json:"sfx_volume"`
	Normalize     bool    `json:"normalize_sounds"`

	PTT struct {
		Key        string `json:"key"`
		Auto       bool   `json:"auto"`
		AutoDetect bool   `json:"auto_detect"`
		LeadMS     int    `json:"lead_ms"`
		TailMS     int    `json:"tail_ms"`
	} `json:"ptt"`

	StartPreset string                `json:"start_preset"`
	SoundsDir   string                `json:"sounds_dir"`
	TTSRate     int                   `json:"tts_rate"`
	Sounds      map[string]sounds.Def `json:"sounds"`
	Hotkeys     map[string]string     `json:"hotkeys"`
	Timers      []timers.Timer        `json:"timers"`
	Rosh        struct {
		Sound     string `json:"sound"`
		AegisWarn int    `json:"aegis_warn"`
		MinWarn   int    `json:"min_warn"`
	} `json:"rosh"`
	Events map[string]string `json:"events"`
	GSI    struct {
		Enabled bool   `json:"enabled"`
		Addr    string `json:"addr"`
		Token   string `json:"token"`
		LowHP   int    `json:"low_hp_percent"`
	} `json:"gsi"`
	VoicePresets map[string][]dsp.Spec `json:"voice_presets"`
	Scripts      map[string]bool       `json:"scripts"`
	UI           struct {
		Port               int  `json:"port"`
		KeepRunningOnClose bool `json:"keep_running_on_close"`
		OpenOnStart        bool `json:"open_on_start"`
	} `json:"ui"`
	SetupDone bool `json:"setup_done"`

	// v2: пульт
	Mic struct {
		Mute   bool    `json:"mute"`
		GateDB float64 `json:"gate_db"` // 0 = гейт выключен
	} `json:"mic"`
	Sources []audio.SourceConfig `json:"sources"`
	Update  struct {
		AutoCheck   bool   `json:"auto_check"`
		AutoInstall bool   `json:"auto_install"`
		Skip        string `json:"skip,omitempty"` // пропустить эту версию
	} `json:"update"`
}

// Save записывает конфиг атомарно (через временный файл) и хранит 5 прошлых версий в backups/.
func Save(path string, c *Config) error {
	c.Version = Version
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	backup(path)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load читает конфиг; поверх значений по умолчанию.
func Load(path string) (*Config, error) {
	var c Config
	if err := json.Unmarshal(DefaultJSON, &c); err != nil {
		return nil, fmt.Errorf("встроенный конфиг: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Карты из дефолта не должны «протекать» в пользовательский конфиг,
	// если пользователь задал их явно — поэтому обнуляем перед разбором.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if _, ok := probe["hotkeys"]; ok {
		c.Hotkeys = nil
	}
	if _, ok := probe["timers"]; ok {
		c.Timers = nil
	}
	if _, ok := probe["events"]; ok {
		c.Events = nil
	}
	if _, ok := probe["scripts"]; ok {
		c.Scripts = nil
	}
	if _, ok := probe["sounds"]; ok {
		c.Sounds = nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if _, ok := probe["version"]; !ok {
		c.Version = 1
	}
	if c.Version < Version {
		migrate(&c, probe)
	}
	return &c, nil
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
	c.Version = Version
}

const keepBackups = 5

func backup(path string) {
	old, err := os.ReadFile(path)
	if err != nil {
		return
	}
	dir := filepath.Join(filepath.Dir(path), "backups")
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	name := filepath.Join(dir, "config-"+time.Now().Format("20060102-150405.000000000")+".json")
	if os.WriteFile(name, old, 0o644) != nil {
		return
	}
	entries, _ := os.ReadDir(dir)
	var list []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config-") {
			list = append(list, e.Name())
		}
	}
	sort.Strings(list)
	for len(list) > keepBackups {
		os.Remove(filepath.Join(dir, list[0]))
		list = list[1:]
	}
}

// Backups — список резервных копий (новые первыми).
func Backups(path string) []string {
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(path), "backups"))
	var out []string
	for i := len(entries) - 1; i >= 0; i-- {
		if strings.HasPrefix(entries[i].Name(), "config-") {
			out = append(out, entries[i].Name())
		}
	}
	return out
}

// Restore возвращает конфиг из резервной копии.
func Restore(path, name string) error {
	if strings.ContainsAny(name, `/\`) || !strings.HasPrefix(name, "config-") {
		return fmt.Errorf("плохое имя копии")
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(path), "backups", name))
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	backup(path)
	return os.WriteFile(path, b, 0o644)
}

// EnsureFile создаёт config.json со значениями по умолчанию, если его нет.
func EnsureFile(path string) (created bool, err error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, os.WriteFile(path, DefaultJSON, 0o644)
}
