// Package sounds — загрузка аудио (WAV/MP3), синтез встроенных звуков и библиотека.
package sounds

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
)

// SampleRate — внутренняя частота всего приложения.
const SampleRate = 48000

// Clip — моно, float32, 48 кГц.
type Clip []float32

// DecodeFile читает WAV или MP3 и приводит к 48 кГц моно.
func DecodeFile(path string) (Clip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return DecodeWAV(data)
	case ".mp3":
		return DecodeMP3(data)
	}
	return nil, fmt.Errorf("формат %s не поддерживается (нужен .wav или .mp3)", filepath.Ext(path))
}

func DecodeMP3(data []byte) (Clip, error) {
	d, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(d)
	if err != nil && len(raw) == 0 {
		return nil, err
	}
	n := len(raw) / 4 // 16 бит, стерео
	mono := make([]float32, n)
	for i := 0; i < n; i++ {
		l := int16(binary.LittleEndian.Uint16(raw[i*4:]))
		r := int16(binary.LittleEndian.Uint16(raw[i*4+2:]))
		mono[i] = (float32(l) + float32(r)) / 65536
	}
	return Resample(mono, d.SampleRate(), SampleRate), nil
}

func DecodeWAV(data []byte) (Clip, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("не WAV файл")
	}
	var format, channels, bits uint16
	var rate uint32
	var pcm []byte
	for p := 12; p+8 <= len(data); {
		id := string(data[p : p+4])
		size := int(binary.LittleEndian.Uint32(data[p+4:]))
		body := data[p+8:]
		if size > len(body) {
			size = len(body)
		}
		body = body[:size]
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("битый fmt")
			}
			format = binary.LittleEndian.Uint16(body[0:])
			channels = binary.LittleEndian.Uint16(body[2:])
			rate = binary.LittleEndian.Uint32(body[4:])
			bits = binary.LittleEndian.Uint16(body[14:])
			if format == 0xFFFE && size >= 26 { // WAVE_FORMAT_EXTENSIBLE
				format = binary.LittleEndian.Uint16(body[24:])
			}
		case "data":
			pcm = body
		}
		p += 8 + size + size%2
	}
	if channels == 0 || rate == 0 || pcm == nil {
		return nil, errors.New("WAV без fmt/data")
	}
	bps := int(bits / 8)
	frame := bps * int(channels)
	n := len(pcm) / frame
	mono := make([]float32, n)
	for i := 0; i < n; i++ {
		var sum float32
		for c := 0; c < int(channels); c++ {
			o := i*frame + c*bps
			var v float32
			switch {
			case format == 3 && bits == 32:
				v = math.Float32frombits(binary.LittleEndian.Uint32(pcm[o:]))
			case format == 1 && bits == 8:
				v = (float32(pcm[o]) - 128) / 128
			case format == 1 && bits == 16:
				v = float32(int16(binary.LittleEndian.Uint16(pcm[o:]))) / 32768
			case format == 1 && bits == 24:
				x := int32(pcm[o]) | int32(pcm[o+1])<<8 | int32(int8(pcm[o+2]))<<16
				v = float32(x) / 8388608
			case format == 1 && bits == 32:
				v = float32(int32(binary.LittleEndian.Uint32(pcm[o:]))) / 2147483648
			default:
				return nil, fmt.Errorf("WAV формат %d/%d бит не поддерживается", format, bits)
			}
			sum += v
		}
		mono[i] = sum / float32(channels)
	}
	return Resample(mono, int(rate), SampleRate), nil
}

// Resample — линейная передискретизация (для звуков-эффектов достаточно).
func Resample(in []float32, from, to int) Clip {
	if from == to || len(in) == 0 {
		return in
	}
	n := int(int64(len(in)) * int64(to) / int64(from))
	out := make([]float32, n)
	step := float64(from) / float64(to)
	for i := range out {
		pos := float64(i) * step
		j := int(pos)
		f := float32(pos - float64(j))
		a := in[j]
		b := a
		if j+1 < len(in) {
			b = in[j+1]
		}
		out[i] = a + (b-a)*f
	}
	return out
}

// EncodeWAV16 — для тестов и экспорта встроенных звуков.
func EncodeWAV16(c Clip) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(c)*2))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(SampleRate))
	w(uint32(SampleRate * 2))
	w(uint16(2))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(len(c) * 2))
	for _, x := range c {
		if x > 1 {
			x = 1
		} else if x < -1 {
			x = -1
		}
		w(int16(x * 32767))
	}
	return b.Bytes()
}
