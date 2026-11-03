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
const Version = 4

type Config struct {
	Version int `json:"version"`
	Devices struct {
		Mic      string `json:"mic"`
		VoiceOut string `json:"voice_out"`
		CableRec string `json:"cable_rec,omitempty"` // «другой конец» кабеля (для тестов и подсказок); "" — авто
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

	// Talk — когда микрофон идёт в эфир: всегда, пока держу кнопку, или по нажатию (вкл/выкл).
	Talk struct {
		Mode      string `json:"mode"`           // always | ptt | toggle
		Key       string `json:"key"`            // "" = кнопка голосового чата игры (ptt.key)
		ReleaseMS int    `json:"release_ms"`     // задержка закрытия после отпускания — не обрезать конец слова
		PressGame bool   `json:"press_game_key"` // сам зажимать кнопку голосового чата игры, пока говорю
	} `json:"talk"`

	StartPreset  string                        `json:"start_preset"`
	PresetParams map[string]map[string]float64 `json:"preset_params"` // ручки эффектов: пресет → ручка → значение
	SoundsDir    string                        `json:"sounds_dir"`
	TTSRate      int                           `json:"tts_rate"`
	TTSVoice     string                        `json:"tts_voice,omitempty"`
	Sounds       map[string]sounds.Def         `json:"sounds"`
	Pad          []string                      `json:"pad"` // порядок плиток на панели
	Hotkeys      map[string]string             `json:"hotkeys"`
	QuickMenu    QuickMenu                     `json:"quick_menu"`
	Wheel        Wheel                         `json:"wheel"`
	Remote       Remote                        `json:"remote"`
	Dota         Dota                          `json:"dota"`
	Timers       []timers.Timer                `json:"timers,omitempty"` // v2; при миграции переезжают в dota.reminders
	Rosh         struct {
		Sound     string `json:"sound,omitempty"` // v2
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
		TrayHintShown      bool `json:"tray_hint_shown,omitempty"`
	} `json:"ui"`
	SetupDone bool `json:"setup_done"`

	// v2: пульт
	Mic struct {
		Mute   bool    `json:"mute"`
		GateDB float64 `json:"gate_db"` // 0 = гейт выключен

		// v4: улучшение голоса до эффекта
		Denoise      bool      `json:"denoise"`       // шумоподавление нейросетью (RNNoise)
		DenoiseLevel float64   `json:"denoise_level"` // сила 0..1 (1 — максимум)
		Timbre       string    `json:"timbre"`        // "" — без обработки | natural | warm | … | custom
		EQ           []float64 `json:"eq,omitempty"`  // свой тембр: дБ по полосам (глубина, бубнение, середина, чёткость, воздух)
		Smooth       float64   `json:"smooth"`        // свой тембр: ровность громкости 0..1
	} `json:"mic"`
	// Background — фон под голос (дождь, костёр… или свой звук).
	Background Background `json:"background"`
	// v4: наборы звуков — свои для каждой игры; переключаются вручную или сами
	Sets      []SoundSet           `json:"sets,omitempty"`
	ActiveSet string               `json:"active_set,omitempty"` // "" — все звуки
	AutoSet   bool                 `json:"auto_set"`             // включать набор игры, когда она на переднем плане
	Sources   []audio.SourceConfig `json:"sources"`
	Update    struct {
		AutoCheck   bool   `json:"auto_check"`
		AutoInstall bool   `json:"auto_install"`
		Skip        string `json:"skip,omitempty"` // пропустить эту версию
	} `json:"update"`
}

// Background — фон под голос.
type Background struct {
	ID          string  `json:"id"`             // "" — нет | rain | storm | wind | fire | sea | space | vinyl | sound:<id>
	Volume      float64 `json:"volume"`         // 0..1
	OnlyTalking bool    `json:"only_talking"`   // слышно только пока вы говорите
	Last        string  `json:"last,omitempty"` // последний фон — его включает действие «фон вкл/выкл»
}

// SoundSet — набор звуков (например, для Dota 2 и для CS2).
type SoundSet struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Emoji  string   `json:"emoji,omitempty"`
	Sounds []string `json:"sounds"`          // звуки набора в порядке панели
	Games  []string `json:"games,omitempty"` // программы игр (dota2.exe): на переднем плане — набор включается сам
}

// QuickMenu — маленькое меню поверх игры по горячей клавише (действие "menu").
type QuickMenu struct {
	Enabled bool     `json:"enabled"`
	Items   []string `json:"items"`   // звуки по порядку; пусто — первые с панели
	Effects []string `json:"effects"` // эффекты голоса на вкладке Tab; пусто — избранные
	Layout  string   `json:"layout"`  // list — список | strip — полоска | grid — сетка 5×2
	// Position — где появляется: center | top | bottom | left | right | top-left | top-right |
	// bottom-left | bottom-right | cursor (у курсора) | custom (своё место: x, y)
	Position       string  `json:"position"`
	X              float64 `json:"x"`                // custom: доля свободного места по горизонтали (0 — у левого края, 1 — у правого)
	Y              float64 `json:"y"`                // custom: по вертикали (0 — вплотную к верху, 1 — к низу)
	Monitor        string  `json:"monitor"`          // game — где игра | cursor — где курсор | primary — основной
	Opacity        float64 `json:"opacity"`          // непрозрачность фона (0.3–1)
	CloseAfterPick bool    `json:"close_after_pick"` // закрывать после выбора
	TimeoutSec     int     `json:"timeout_sec"`      // само закроется через N секунд бездействия (0 — нет)
	ShowEffects    bool    `json:"show_effects"`     // вкладка с эффектами голоса
	Scale          float64 `json:"scale"`            // размер (1 = обычный)
}

// WheelSlots — секторов в колесе звуков.
const WheelSlots = 8

// Wheel — колесо звуков (как колесо чата в Dota 2): зажал клавишу — вокруг курсора 8 секторов,
// повёл мышь к нужному — отпустил (действие "wheel").
type Wheel struct {
	Enabled bool `json:"enabled"`
	// Items — 8 секторов по часовой стрелке от верхнего: sound:<id> | preset:<id> | stop | mic_mute | "" (пусто).
	// Пустой список — первые 8 звуков с панели.
	Items   []string `json:"items"`
	Center  string   `json:"center"`  // cursor — у курсора | screen — по центру экрана
	Scale   float64  `json:"scale"`   // размер (1 = обычный)
	Opacity float64  `json:"opacity"` // непрозрачность секторов (0.3–1)
}

// Remote — пульт с телефона: плитки звуков в браузере телефона по домашней сети (Wi-Fi).
type Remote struct {
	Enabled     bool   `json:"enabled"`
	Port        int    `json:"port"`         // 0 — порт интерфейса + 1 (обычно 3002)
	Key         string `json:"key"`          // секрет в адресе пульта; сменить — старые ссылки перестанут работать
	Sounds      string `json:"sounds"`       // pad — все звуки панели | menu — как в быстром меню
	ShowEffects bool   `json:"show_effects"` // вкладка «Голос» с эффектами
	ShowMic     bool   `json:"show_mic"`     // кнопка «выключить микрофон»
}

// Dota — модуль Dota 2: голосовые напоминания по игровому времени, Рошан, кнопки-оповещения.
type Dota struct {
	Enabled   bool       `json:"enabled"`
	VoiceTo   string     `json:"voice_to"` // кто слышит напоминания: monitor (только я) | both | voice
	Volume    float64    `json:"volume"`
	Rosh      bool       `json:"rosh"` // напоминания об аегисе и респауне Рошана
	Reminders []Reminder `json:"reminders"`
}

// Reminder — напоминание по игровому времени («Руна мудрости через 20 секунд»).
type Reminder struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	At      []int  `json:"at,omitempty"`    // разовые моменты, сек игрового времени
	Start   int    `json:"start,omitempty"` // первый раз
	Every   int    `json:"every,omitempty"` // потом каждые N сек
	Until   int    `json:"until,omitempty"` // до (0 — до конца игры)
	Skip    []int  `json:"skip,omitempty"`
	Before  int    `json:"before"`          // за сколько секунд предупредить
	Say     string `json:"say,omitempty"`   // фраза; пусто — «<название> через N секунд»
	Sound   string `json:"sound,omitempty"` // сигнал перед фразой: "" — колокольчик, "none" — без сигнала, id звука или builtin:x
	To      string `json:"to,omitempty"`    // кто слышит; "" — как в dota.voice_to
}

// UnmarshalJSON разбирает напоминание «с нуля»: encoding/json при разборе в существующий срез
// дописывает поля поверх старых элементов, и напоминание пользователя получало бы недостающие поля
// от напоминания по умолчанию с тем же номером.
func (r *Reminder) UnmarshalJSON(b []byte) error {
	type plain Reminder
	var x plain
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	*r = Reminder(x)
	return nil
}

// UnmarshalJSON — набор «с нуля» (см. Reminder.UnmarshalJSON).
func (s *SoundSet) UnmarshalJSON(b []byte) error {
	type plain SoundSet
	var x plain
	if err := json.Unmarshal(b, &x); err != nil {
		return err
	}
	*s = SoundSet(x)
	return nil
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
	if _, ok := probe["preset_params"]; ok {
		c.PresetParams = nil
	}
	if _, ok := probe["voice_presets"]; ok {
		c.VoicePresets = nil
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
	c.Fix()
	return &c, nil
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
