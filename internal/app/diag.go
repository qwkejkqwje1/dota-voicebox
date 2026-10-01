package app

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// Diagnoser — возможности движка для тестов звука (есть у audio.Engine).
type Diagnoser interface {
	RecordMic(time.Duration) ([]float32, error)
	ResetTestPeaks()
	TestPeaks() (voice, monitor float32)
	CaptureCable(time.Duration, func()) ([]float32, time.Time, string, error)
}

// TestResult — результат одного теста для интерфейса.
type TestResult struct {
	ID      string         `json:"id"`
	Verdict string         `json:"verdict"` // ok | warn | fail
	Summary string         `json:"summary"`
	Details []string       `json:"details,omitempty"`
	Ask     string         `json:"ask,omitempty"` // вопрос пользователю («Слышали сигнал?»)
	Stats   *audio.Stats   `json:"stats,omitempty"`
	Stats2  *audio.Stats   `json:"stats2,omitempty"`
	Suggest map[string]any `json:"suggest,omitempty"`
}

var testBusy atomic.Bool

const sr = sounds.SampleRate

func (a *App) diag() (Diagnoser, error) {
	d, ok := a.out.(Diagnoser)
	if !ok {
		return nil, errors.New("тесты недоступны")
	}
	if !a.out.Running() {
		return nil, errors.New("аудио не запущено — сначала пройдите «Настройку»")
	}
	return d, nil
}

// RunTest выполняет тест: monitor, cable, mic, voice, team, ptt.
func (a *App) RunTest(id string, sec float64, preset string) (*TestResult, error) {
	if !testBusy.CompareAndSwap(false, true) {
		return nil, errors.New("уже идёт другой тест")
	}
	defer testBusy.Store(false)
	if sec <= 0 || sec > 10 {
		sec = 3
	}
	if id == "ptt" {
		return a.testPTT()
	}
	d, err := a.diag()
	if err != nil {
		return nil, err
	}
	switch id {
	case "monitor":
		return a.testMonitor(d)
	case "cable":
		return a.testCable(d)
	case "mic":
		return a.testMic(d, sec)
	case "voice":
		return a.testVoice(d, sec, preset)
	case "team":
		return a.testTeam(d, sec)
	}
	return nil, fmt.Errorf("неизвестный тест %q", id)
}

func (a *App) testMonitor(d Diagnoser) (*TestResult, error) {
	d.ResetTestPeaks()
	a.out.Monitor().Add("test", audio.Tone(880, 0.25, 0.35, sr), 1, "interrupt")
	time.Sleep(350 * time.Millisecond)
	a.out.Monitor().Add("test", audio.Tone(1320, 0.35, 0.35, sr), 1, "interrupt")
	time.Sleep(600 * time.Millisecond)
	_, mon := d.TestPeaks()
	st := a.out.Status()
	r := &TestResult{ID: "monitor"}
	if mon < 0.005 {
		r.Verdict = "fail"
		r.Summary = "Звук не дошёл до наушников"
		if a.Config().MonitorVolume == 0 {
			r.Details = append(r.Details, "Громкость «Звуки для вас» = 0 — поднимите её на главной")
		}
		r.Details = append(r.Details, "Проверьте устройство «Наушники» на вкладке «Настройка»")
		return r, nil
	}
	r.Verdict = "ok"
	r.Summary = fmt.Sprintf("Сигнал отправлен в «%s» (уровень %.0f dB)", st.Monitor, audio.DB(float64(mon)))
	r.Ask = "Слышали два коротких сигнала? Если нет — проверьте громкость Windows и выбранные наушники."
	return r, nil
}

func (a *App) testCable(d Diagnoser) (*TestResult, error) {
	var playAt time.Time
	tone := audio.Tone(1000, 0.4, 0.3, sr)
	buf, start, name, err := d.CaptureCable(1600*time.Millisecond, func() {
		playAt = time.Now()
		a.out.Voice().Add("test", tone, 1, "interrupt")
	})
	r := &TestResult{ID: "cable"}
	if err != nil {
		r.Verdict, r.Summary = "fail", "Не удалось слушать кабель: "+err.Error()
		r.Details = []string{"Убедитесь, что установлен VB-Audio Virtual Cable (вкладка «Настройка»)"}
		return r, nil
	}
	onset, lvl := audio.ToneOnset(buf, sr, 1000)
	if onset < 0 {
		r.Verdict = "fail"
		r.Summary = fmt.Sprintf("Тестовый тон не пришёл в «%s» — Dota не услышит звуки и голос", name)
		r.Details = []string{
			"Проверьте, что «Голос в Dota» = CABLE Input (вкладка «Настройка»)",
			"Windows → Звук → Запись → CABLE Output: громкость не 0 и устройство включено",
		}
		return r, nil
	}
	lat := start.Add(time.Duration(onset * float64(time.Second))).Sub(playAt)
	r.Verdict = "ok"
	r.Summary = fmt.Sprintf("Кабель работает: тон дошёл до «%s», уровень %.0f dB, задержка ≈ %d мс", name, lvl, lat.Milliseconds())
	r.Details = []string{"В Dota 2 → Настройки → Звук → «Устройство записи» должно быть: " + name}
	if lvl < -30 {
		r.Verdict = "warn"
		r.Details = append(r.Details, "Уровень низкий — проверьте громкость CABLE Output в Windows (Запись → Свойства → Уровни = 100)")
	}
	if lat > 250*time.Millisecond {
		r.Verdict = "warn"
		r.Details = append(r.Details, "Большая задержка — закройте лишние программы, работающие со звуком")
	}
	return r, nil
}

func micVerdict(st audio.Stats, gain float64) (string, string, []string) {
	var det []string
	switch {
	case st.Silent:
		return "fail", "Микрофон молчит", []string{
			"Проверьте устройство «Микрофон» на вкладке «Настройка» и кнопку mute на гарнитуре",
			"Windows → Параметры → Конфиденциальность → Микрофон: разрешить классическим приложениям",
		}
	case st.ClipPct > 0.1:
		det = append(det, fmt.Sprintf("Перегруз %.2f%% — звук будет хрипеть. Уменьшите усиление", st.ClipPct))
	}
	speech := st.SpeechDB + 20*math.Log10(math.Max(gain, 0.01))
	verdict, sum := "ok", fmt.Sprintf("Микрофон работает: речь %.0f dB, фон %.0f dB", st.SpeechDB, st.NoiseDB)
	if speech < -32 {
		verdict = "warn"
		det = append(det, "Тихо: команда будет слышать вас плохо — увеличьте «Усиление микрофона»")
	}
	if st.NoiseDB > -45 {
		verdict = "warn"
		det = append(det, "Сильный фоновый шум — помогут пресеты «Рация» или шумодав Windows/NVIDIA Broadcast")
	}
	if st.ClipPct > 0.1 {
		verdict = "warn"
	}
	if st.SpeechDB-st.NoiseDB < 12 {
		det = append(det, "Речь почти не громче фона — вы говорили во время записи?")
	}
	return verdict, sum, det
}

func (a *App) testMic(d Diagnoser, sec float64) (*TestResult, error) {
	buf, err := d.RecordMic(time.Duration(sec * float64(time.Second)))
	if err != nil {
		return nil, err
	}
	st := audio.Analyze(buf, sr)
	gain := a.Config().MicGain
	r := &TestResult{ID: "mic", Stats: &st}
	r.Verdict, r.Summary, r.Details = micVerdict(st, gain)
	if st.SuggestGain > 0 && math.Abs(st.SuggestGain-gain) >= 0.2 {
		r.Suggest = map[string]any{"mic_gain": st.SuggestGain}
		r.Details = append(r.Details, fmt.Sprintf("Рекомендуемое усиление: %.1f (сейчас %.1f)", st.SuggestGain, gain))
	}
	return r, nil
}

func (a *App) testVoice(d Diagnoser, sec float64, preset string) (*TestResult, error) {
	if preset == "" {
		preset = a.Preset()
	}
	a.mu.Lock()
	specs, ok := a.presets[preset]
	a.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("нет пресета %q", preset)
	}
	buf, err := d.RecordMic(time.Duration(sec * float64(time.Second)))
	if err != nil {
		return nil, err
	}
	chain, err := dsp.BuildChain(preset, specs, sr)
	if err != nil {
		return nil, err
	}
	g := float32(a.Config().MicGain)
	orig := make(sounds.Clip, len(buf))
	proc := make(sounds.Clip, len(buf))
	for i, x := range buf {
		orig[i] = x * g
		proc[i] = x * g
	}
	// обрабатываем блоками, как в реальном аудиопотоке
	for i := 0; i < len(proc); i += 480 {
		chain.Process(proc[i:min(i+480, len(proc))])
	}
	st1, st2 := audio.Analyze(orig, sr), audio.Analyze(proc, sr)
	a.out.Monitor().Add("test_a", orig, 1, "interrupt")
	go func() {
		time.Sleep(time.Duration(float64(len(orig))/sr*float64(time.Second)) + 400*time.Millisecond)
		a.out.Monitor().Add("test_b", proc, 1, "interrupt")
	}()
	r := &TestResult{ID: "voice", Stats: &st1, Stats2: &st2, Verdict: "ok"}
	r.Summary = fmt.Sprintf("Сейчас в наушниках: сначала ваш голос как есть, затем через пресет «%s»", preset)
	if st1.Silent {
		r.Verdict, r.Summary = "fail", "Микрофон молчит — нечего обрабатывать"
	}
	r.Details = []string{fmt.Sprintf("Без эффекта: речь %.0f dB · с эффектом: речь %.0f dB, пик %.0f dB", st1.SpeechDB, st2.SpeechDB, st2.PeakDB)}
	if st2.ClipPct > 0.5 {
		r.Verdict = "warn"
		r.Details = append(r.Details, "Пресет перегружает звук — уменьшите усиление микрофона")
	}
	return r, nil
}

func (a *App) testTeam(d Diagnoser, sec float64) (*TestResult, error) {
	buf, _, name, err := d.CaptureCable(time.Duration(sec*float64(time.Second)), nil)
	r := &TestResult{ID: "team"}
	if err != nil {
		r.Verdict, r.Summary = "fail", "Не удалось слушать кабель: "+err.Error()
		return r, nil
	}
	st := audio.Analyze(buf, sr)
	r.Stats = &st
	a.out.Monitor().Add("test_team", sounds.Clip(buf), 1, "interrupt")
	if st.Silent {
		r.Verdict = "fail"
		r.Summary = fmt.Sprintf("В «%s» тишина — команда вас не услышит", name)
		r.Details = []string{"Запустите тест «Микрофон», затем «Кабель»"}
		return r, nil
	}
	r.Verdict = "ok"
	r.Summary = fmt.Sprintf("Воспроизвожу запись с «%s» — ровно так вас слышит Dota (до сжатия голосового чата)", name)
	r.Details = []string{fmt.Sprintf("Речь %.0f dB, фон %.0f dB, пик %.0f dB", st.SpeechDB, st.NoiseDB, st.PeakDB)}
	return r, nil
}

func (a *App) testPTT() (*TestResult, error) {
	c := a.Config()
	r := &TestResult{ID: "ptt"}
	if c.PTT.Key == "" {
		r.Verdict, r.Summary = "fail", "Кнопка голосового чата не задана"
		r.Details = []string{"Вкладка «Настройка» → «Кнопка голосового чата» или назначьте её в Dota 2 и нажмите «Найти»"}
		return r, nil
	}
	k, err := keys.Parse(c.PTT.Key)
	if err != nil {
		return nil, err
	}
	if a.pttOwned.Load() {
		return nil, errors.New("сейчас кнопку уже держит программа (играет звук)")
	}
	if err := winapi.PressKey(k.VK, true); err != nil {
		r.Verdict, r.Summary = "fail", "Не удалось нажать кнопку: "+err.Error()
		return r, nil
	}
	time.Sleep(1500 * time.Millisecond)
	winapi.PressKey(k.VK, false)
	r.Verdict = "ok"
	r.Summary = fmt.Sprintf("Кнопка %s была зажата 1,5 с", c.PTT.Key)
	r.Ask = "В Dota 2 (в матче или демо-режиме) должна была загореться иконка микрофона. Если Dota запущена от администратора — запустите VoiceBox тоже от администратора."
	return r, nil
}
