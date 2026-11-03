package audio

import (
	"errors"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Запись нового звука с микрофона: голос после усиления, шумоподавления и гейта,
// но без эффектов, звуков и фона (эффект при желании накладывается при сохранении).

// StartRecord начинает запись не длиннее max.
func (e *Engine) StartRecord(max time.Duration) error {
	if !e.Running() {
		return errors.New("аудио не запущено")
	}
	r := &recorder{buf: make([]float32, int(max.Seconds()*sounds.SampleRate))}
	if !e.recTap.CompareAndSwap(nil, r) {
		return errors.New("запись уже идёт")
	}
	return nil
}

// RecordStatus — идёт ли запись, сколько секунд записано, кончилось ли место.
func (e *Engine) RecordStatus() (active bool, sec float64, full bool) {
	r := e.recTap.Load()
	if r == nil {
		return false, 0, false
	}
	return true, float64(r.pos.Load()) / sounds.SampleRate, r.full()
}

// StopRecord останавливает запись и возвращает её (nil — запись не шла).
func (e *Engine) StopRecord() []float32 {
	r := e.recTap.Swap(nil)
	if r == nil {
		return nil
	}
	time.Sleep(30 * time.Millisecond) // аудиопоток дописывает последний блок
	return r.buf[:r.pos.Load()]
}
