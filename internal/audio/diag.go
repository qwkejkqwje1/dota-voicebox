package audio

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// recorder — запись из аудиопотока без блокировок.
type recorder struct {
	buf []float32
	pos atomic.Int64
}

func (r *recorder) write(p []float32) {
	pos := r.pos.Load()
	n := copy(r.buf[pos:], p)
	r.pos.Store(pos + int64(n))
}

func (r *recorder) full() bool { return int(r.pos.Load()) >= len(r.buf) }

func (r *recorder) wait(d time.Duration) []float32 {
	dead := time.Now().Add(d + time.Second)
	for !r.full() && time.Now().Before(dead) {
		time.Sleep(20 * time.Millisecond)
	}
	return r.buf[:r.pos.Load()]
}

// RecordMic записывает «сырой» микрофон (до усиления и эффектов).
func (e *Engine) RecordMic(d time.Duration) ([]float32, error) {
	if !e.Running() {
		return nil, errors.New("аудио не запущено")
	}
	r := &recorder{buf: make([]float32, int(d.Seconds()*sounds.SampleRate))}
	if !e.rawRec.CompareAndSwap(nil, r) {
		return nil, errors.New("уже идёт запись")
	}
	defer e.rawRec.Store(nil)
	return r.wait(d), nil
}

// ResetTestPeaks / TestPeaks — пиковые уровни выходов с момента сброса (для тестов).
func (e *Engine) ResetTestPeaks() { e.tpOut.Store(0); e.tpMon.Store(0) }
func (e *Engine) TestPeaks() (voice, monitor float32) {
	return e.tpOut.Load(), e.tpMon.Load()
}

// cableCapturePair — какое устройство ввода соответствует выбранному кабелю.
func cableCapturePair(voiceOut string) []string {
	n := strings.ToLower(voiceOut)
	switch {
	case strings.Contains(n, "voicemeeter aux"):
		return []string{"voicemeeter out b2", "voicemeeter aux output"}
	case strings.Contains(n, "voicemeeter"):
		return []string{"voicemeeter out b1", "voicemeeter output"}
	case strings.Contains(n, "cable input"):
		// "CABLE Input (VB-Audio Virtual Cable)" → "CABLE Output (VB-Audio Virtual Cable)"
		return []string{strings.Replace(n, "cable input", "cable output", 1), "cable output"}
	}
	return []string{"cable output"}
}

// CaptureCable записывает то, что приходит на «другой конец» виртуального кабеля —
// ровно то, что слышит Dota 2. during вызывается через 300 мс после старта записи.
// Возвращает запись, момент начала записи и имя устройства.
func (e *Engine) CaptureCable(d time.Duration, during func()) ([]float32, time.Time, string, error) {
	st := e.Status()
	if !st.Running {
		return nil, time.Time{}, "", errors.New("аудио не запущено")
	}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, time.Time{}, "", err
	}
	defer func() { ctx.Uninit(); ctx.Free() }()
	ds, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, time.Time{}, "", err
	}
	var id *malgo.DeviceID
	var name string
	for _, want := range cableCapturePair(st.VoiceOut) {
		for i := range ds {
			if strings.Contains(strings.ToLower(ds[i].Name()), want) {
				x := ds[i].ID
				id, name = &x, ds[i].Name()
				break
			}
		}
		if id != nil {
			break
		}
	}
	if id == nil {
		return nil, time.Time{}, "", fmt.Errorf("не найдено устройство записи кабеля (ищу %q)", cableCapturePair(st.VoiceOut)[0])
	}
	r := &recorder{buf: make([]float32, int(d.Seconds()*sounds.SampleRate))}
	var started atomic.Int64
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.SampleRate = sounds.SampleRate
	cfg.PeriodSizeInMilliseconds = 10
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.Capture.DeviceID = id.Pointer()
	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: func(_, in []byte, frames uint32) {
		if started.Load() == 0 {
			// момент первого сэмпла этого буфера
			started.Store(time.Now().Add(-time.Duration(frames) * time.Second / sounds.SampleRate).UnixNano())
		}
		r.write(f32(in))
	}})
	if err != nil {
		return nil, time.Time{}, name, fmt.Errorf("%s: %w", name, err)
	}
	defer dev.Uninit()
	if err := dev.Start(); err != nil {
		return nil, time.Time{}, name, err
	}
	if during != nil {
		go func() {
			for i := 0; i < 100 && started.Load() == 0; i++ {
				time.Sleep(10 * time.Millisecond)
			}
			time.Sleep(300 * time.Millisecond)
			during()
		}()
	}
	buf := r.wait(d)
	dev.Stop()
	return buf, time.Unix(0, started.Load()), name, nil
}
