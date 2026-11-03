// Package rnnoise — шумоподавление голоса нейросетью RNNoise (Xiph.Org, BSD-3, см. COPYING).
// Исходники C лежат рядом и собираются вместе с программой; веса модели встроены (model.bin.gz).
// Работает на 48 кГц кадрами по 10 мс; около 2% одного ядра процессора.
package rnnoise

/*
#cgo CFLAGS: -O2 -DUSE_WEIGHTS_FILE -DDISABLE_DEBUG_FLOAT -w
#cgo LDFLAGS: -lm
#include <stdlib.h>
#include "rnnoise.h"
*/
import "C"

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"io"
	"sync"
	"unsafe"
)

//go:embed model.bin.gz
var modelGz []byte

// FrameSize — отсчётов в кадре (10 мс при 48 кГц).
const FrameSize = 480

// Delay — собственная задержка сети: выход кадра соответствует входу на 2 кадра раньше.
const Delay = 2 * FrameSize

var (
	modelOnce sync.Once
	model     *C.RNNModel
	modelErr  error
)

// loadModel распаковывает веса один раз; память под них живёт до конца программы.
func loadModel() (*C.RNNModel, error) {
	modelOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(modelGz))
		if err != nil {
			modelErr = err
			return
		}
		data, err := io.ReadAll(zr)
		if err != nil {
			modelErr = err
			return
		}
		p := C.CBytes(data)
		model = C.rnnoise_model_from_buffer(p, C.int(len(data)))
		if model == nil {
			C.free(p)
			modelErr = errors.New("rnnoise: не удалось загрузить модель")
		}
	})
	return model, modelErr
}

// State — один поток шумоподавления (не для одновременного использования из разных горутин).
type State struct {
	st      *C.DenoiseState
	in, out [FrameSize]C.float
}

// NewState создаёт поток шумоподавления.
func NewState() (*State, error) {
	m, err := loadModel()
	if err != nil {
		return nil, err
	}
	st := C.rnnoise_create(m)
	if st == nil {
		return nil, errors.New("rnnoise: не удалось создать состояние")
	}
	return &State{st: st}, nil
}

// Close освобождает память потока.
func (s *State) Close() {
	if s != nil && s.st != nil {
		C.rnnoise_destroy(s.st)
		s.st = nil
	}
}

// Frame обрабатывает ровно FrameSize отсчётов (-1..1) на месте и возвращает вероятность речи 0..1.
// Выход задержан на Delay отсчётов относительно входа.
func (s *State) Frame(buf []float32) float32 {
	_ = buf[FrameSize-1]
	for i := 0; i < FrameSize; i++ {
		s.in[i] = C.float(buf[i] * 32768)
	}
	vad := C.rnnoise_process_frame(s.st, (*C.float)(unsafe.Pointer(&s.out[0])), (*C.float)(unsafe.Pointer(&s.in[0])))
	for i := 0; i < FrameSize; i++ {
		buf[i] = float32(s.out[i]) / 32768
	}
	return float32(vad)
}
