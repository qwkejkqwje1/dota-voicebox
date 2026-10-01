// Package config — загрузка config.json (с комментариями не работает: чистый JSON).
package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/timers"
)

//go:embed default.json
var DefaultJSON []byte

type Config struct {
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
	UI           struct {
		Port               int  `json:"port"`
		KeepRunningOnClose bool `json:"keep_running_on_close"`
		OpenOnStart        bool `json:"open_on_start"`
	} `json:"ui"`
	SetupDone bool `json:"setup_done"`
}

// Save записывает конфиг атомарно (через временный файл).
func Save(path string, c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
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
	if _, ok := probe["sounds"]; ok {
		c.Sounds = nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
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
