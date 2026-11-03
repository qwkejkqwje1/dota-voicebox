package app

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Запись звуков с микрофона, обрезка файлов и звуки на события Dota 2.

// Recorder — запись голоса для новых звуков (есть у audio.Engine).
type Recorder interface {
	StartRecord(max time.Duration) error
	RecordStatus() (active bool, sec float64, full bool)
	StopRecord() []float32
}

// RecMax — самая длинная запись, сек.
const RecMax = 60

type recState struct {
	mu   sync.Mutex
	clip sounds.Clip // последняя запись — ждёт сохранения
	edit struct {    // файл, открытый в окне обрезки
		id, path string
		raw      sounds.Clip
	}
}

// Wave — волна и границы звука для окна обрезки.
type Wave struct {
	Duration float64    `json:"duration"`
	Peaks    []float32  `json:"peaks"`
	Start    float64    `json:"start"`
	End      float64    `json:"end"`
	Sound    [2]float64 `json:"sound"`            // где сам звук, без тишины по краям
	Silent   bool       `json:"silent,omitempty"` // записалась тишина
}

func waveOf(c sounds.Clip, start, end float64) *Wave {
	dur := float64(len(c)) / sr
	if end <= 0 || end > dur {
		end = dur
	}
	s, e := sounds.FindSilence(c)
	pk := sounds.Peaks(c, 800)
	var top float32
	for i, v := range pk {
		top = max(top, v)
		pk[i] = float32(math.Round(float64(v)*1000) / 1000)
	}
	return &Wave{Duration: round3(dur), Peaks: pk, Start: round3(start), End: round3(end), Sound: [2]float64{s, e}, Silent: top < 0.003}
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func (a *App) recorder() (Recorder, error) {
	if r, ok := a.out.(Recorder); ok {
		return r, nil
	}
	return nil, fmt.Errorf("запись с микрофона недоступна")
}

// RecStart — начать запись нового звука с микрофона.
func (a *App) RecStart() error {
	r, err := a.recorder()
	if err != nil {
		return err
	}
	if !a.out.Running() {
		return fmt.Errorf("аудио не запущено — см. страницу «Проверка»")
	}
	if a.Config().Mic.Mute {
		return fmt.Errorf("микрофон выключен — включите его, чтобы записать звук")
	}
	return r.StartRecord(RecMax * time.Second)
}

// RecStatus — идёт ли запись и сколько записано (для индикатора).
func (a *App) RecStatus() map[string]any {
	r, err := a.recorder()
	if err != nil {
		return map[string]any{"active": false, "max": RecMax}
	}
	on, sec, full := r.RecordStatus()
	return map[string]any{"active": on, "sec": round3(sec), "full": full, "max": RecMax}
}

// RecStop заканчивает запись: она ждёт сохранения, в ответе — волна для обрезки.
func (a *App) RecStop() (*Wave, error) {
	r, err := a.recorder()
	if err != nil {
		return nil, err
	}
	buf := r.StopRecord()
	if buf == nil {
		return nil, fmt.Errorf("запись не шла")
	}
	if len(buf) < sr/5 {
		return nil, fmt.Errorf("запись слишком короткая")
	}
	c := sounds.Clip(buf)
	sounds.PeakNormalize(c, 0.89, 20) // тихий микрофон — не тихий звук
	a.rec.mu.Lock()
	a.rec.clip = c
	a.rec.mu.Unlock()
	return waveOf(c, 0, 0), nil
}

// recClip — кусок последней записи (с эффектом голоса, если нужно).
func (a *App) recClip(start, end float64, effect bool) (sounds.Clip, error) {
	a.rec.mu.Lock()
	c := a.rec.clip
	a.rec.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("записи нет — запишите звук ещё раз")
	}
	c = sounds.TrimClip(c, start, end)
	if effect {
		c = a.withVoiceFX(c)
	}
	return c, nil
}

// withVoiceFX пропускает запись через тембр и текущий эффект голоса — как её услышала бы команда.
func (a *App) withVoiceFX(c sounds.Clip) sounds.Clip {
	var chains []*dsp.Chain
	if specs := TimbreSpecs(a.Config()); specs != nil {
		if ch, err := dsp.BuildChain("timbre", specs, sr); err == nil {
			chains = append(chains, ch)
		}
	}
	a.mu.Lock()
	name := a.preset
	specs, ok := a.presets[name]
	a.mu.Unlock()
	if ok {
		if ch, err := dsp.BuildChain(name, specs, sr); err == nil {
			chains = append(chains, ch)
		}
	}
	// полсекунды тишины в конце — чтобы не оборвать хвост эха
	out := append(append(sounds.Clip(nil), c...), make(sounds.Clip, sr/2)...)
	for i := 0; i < len(out); i += 480 { // блоками, как в аудиопотоке
		b := out[i:min(i+480, len(out))]
		for _, ch := range chains {
			ch.Process(b)
		}
	}
	n := len(out)
	for n > len(c) && math.Abs(float64(out[n-1])) < 0.001 {
		n--
	}
	out = out[:n]
	for i, x := range out {
		out[i] = max(-1, min(1, x))
	}
	return out
}

// playPreview — послушать самому (как прозвучит на панели).
func (a *App) playPreview(c sounds.Clip, vol float32) error {
	if a.Config().Normalize {
		c = sounds.Normalize(c)
	}
	return a.playClip("preview", c, vol, sounds.ModeInterrupt, sounds.BusMonitor)
}

// RecPlay — послушать кусок записи (только вы).
func (a *App) RecPlay(start, end float64, effect bool) error {
	c, err := a.recClip(start, end, effect)
	if err != nil {
		return err
	}
	return a.playPreview(c, 1)
}

// RecSave сохраняет кусок записи новым звуком на панели.
func (a *App) RecSave(name string, start, end float64, effect bool) (string, error) {
	c, err := a.recClip(start, end, effect)
	if err != nil {
		return "", err
	}
	if name = strings.TrimSpace(name); name == "" {
		name = "Запись " + time.Now().Format("15:04")
	}
	id, err := a.importSound(name, sounds.EncodeWAV16(c), ".wav", "🎙️")
	if err != nil {
		return "", err
	}
	a.RecDiscard()
	return id, nil
}

// RecDiscard останавливает запись и забывает её (окно закрыто без сохранения).
func (a *App) RecDiscard() {
	if r, err := a.recorder(); err == nil {
		r.StopRecord()
	}
	a.rec.mu.Lock()
	a.rec.clip = nil
	a.rec.mu.Unlock()
}

// ---------- обрезка готовых звуков ----------

// trimTarget — описание звука и путь к его файлу. Обрезать можно звук из одного файла.
func (a *App) trimTarget(id string) (sounds.Def, string, error) {
	d, ok := a.Config().Sounds[id]
	if !ok {
		a.mu.Lock()
		lib := a.lib
		a.mu.Unlock()
		var s *sounds.Sound
		if lib != nil {
			s = lib.Get(id)
		}
		if s == nil {
			return d, "", fmt.Errorf("звук %q не найден", id)
		}
		d = sounds.Def{Files: s.Files}
	}
	if !sounds.Trimmable(d.Files) {
		return d, "", fmt.Errorf("обрезать можно звук из одного файла — не встроенный и не фразу")
	}
	p := d.Files[0]
	if !filepath.IsAbs(p) {
		p = filepath.Join(a.BaseDir, p)
	}
	return d, p, nil
}

// editRaw — файл звука целиком (без обрезки и выравнивания громкости).
func (a *App) editRaw(id string) (sounds.Clip, sounds.Def, error) {
	d, p, err := a.trimTarget(id)
	if err != nil {
		return nil, d, err
	}
	a.rec.mu.Lock()
	e := a.rec.edit
	a.rec.mu.Unlock()
	if e.id == id && e.path == p && e.raw != nil {
		return e.raw, d, nil
	}
	raw, err := sounds.DecodeFile(p)
	if err != nil {
		return nil, d, err
	}
	a.rec.mu.Lock()
	a.rec.edit.id, a.rec.edit.path, a.rec.edit.raw = id, p, raw
	a.rec.mu.Unlock()
	return raw, d, nil
}

// SoundWave — волна звука для окна обрезки.
func (a *App) SoundWave(id string) (*Wave, error) {
	a.WaveDone() // файл могли заменить — читаем заново
	raw, d, err := a.editRaw(id)
	if err != nil {
		return nil, err
	}
	return waveOf(raw, d.Start, d.End), nil
}

// WaveDone — окно обрезки закрыто: файл в памяти больше не нужен.
func (a *App) WaveDone() {
	a.rec.mu.Lock()
	a.rec.edit.id, a.rec.edit.path, a.rec.edit.raw = "", "", nil
	a.rec.mu.Unlock()
}

// PlayRange — послушать кусок звука (только вы).
func (a *App) PlayRange(id string, start, end float64) error {
	raw, _, err := a.editRaw(id)
	if err != nil {
		return err
	}
	vol := float32(1)
	a.mu.Lock()
	if a.lib != nil {
		if s := a.lib.Get(id); s != nil && s.Volume > 0 {
			vol = s.Volume
		}
	}
	a.mu.Unlock()
	return a.playPreview(sounds.TrimClip(raw, start, end), vol)
}

// TrimSound запоминает обрезку звука: файл не меняется, её всегда можно сбросить (0, 0).
func (a *App) TrimSound(id string, start, end float64) error {
	raw, d, err := a.editRaw(id)
	if err != nil {
		return err
	}
	dur := float64(len(raw)) / sr
	start, end = round3(math.Max(0, start)), round3(end)
	if start < 0.001 {
		start = 0
	}
	if end <= 0 || end >= dur-0.001 {
		end = 0
	}
	to := dur
	if end > 0 {
		to = end
	}
	if to-start < sounds.MinTrim {
		return fmt.Errorf("слишком короткий кусок — оставьте хотя бы 0,05 с")
	}
	a.WaveDone()
	return a.UpdateConfig(func(c *config.Config) {
		if cur, ok := c.Sounds[id]; ok { // свежие настройки звука — не затираем то, что поменяли рядом
			d = cur
		}
		d.Start, d.End = start, end
		c.Sounds[id] = d
	})
}

// ---------- звуки на события Dota 2 ----------

var evName = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// SetEventSound ставит звук на игровое событие Dota 2 (victory, defeat, kill…); sound "" — убрать.
// to: both | voice | monitor | "" (как в настройках звука).
func (a *App) SetEventSound(ev, sound, to string) error {
	if !evName.MatchString(ev) {
		return fmt.Errorf("неизвестное событие %q", ev)
	}
	ref := ""
	if sound = strings.TrimSpace(sound); sound != "" {
		a.mu.Lock()
		lib := a.lib
		a.mu.Unlock()
		if lib == nil || lib.Get(sound) == nil {
			return fmt.Errorf("звук %q не найден", sound)
		}
		ref = sound
		switch to {
		case "":
		case "both", "voice", "monitor":
			ref += "@" + to
		default:
			return fmt.Errorf("кто слышит: %q — нужно both, voice или monitor", to)
		}
	}
	return a.saveQuiet(func(c *config.Config) {
		if c.Events == nil {
			c.Events = map[string]string{}
		}
		if ref == "" {
			delete(c.Events, ev)
		} else {
			c.Events[ev] = ref
		}
	})
}
