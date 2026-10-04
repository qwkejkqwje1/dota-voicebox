package audio

import (
	"math"
	"math/cmplx"
)

// Viz считает кадр визуализации: форма волны (min/max на столбец) и спектр
// в логарифмической шкале частот. Буферы переиспользуются.
type Viz struct {
	N       int // размер БПФ (степень двойки)
	Cols    int // столбцов осциллограммы
	Bands   int // полос спектра
	samples []float32
	win     []float64
	buf     []complex128
	edges   []int
	smooth  []float64
}

func NewViz(n, cols, bands int) *Viz {
	v := &Viz{N: n, Cols: cols, Bands: bands, samples: make([]float32, n), win: make([]float64, n), buf: make([]complex128, n), smooth: make([]float64, bands)}
	for i := range v.win {
		v.win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	// границы полос: 40 Гц … 16 кГц логарифмически
	lo, hi := 40.0, 16000.0
	binHz := float64(48000) / float64(n)
	v.edges = make([]int, bands+1)
	for b := 0; b <= bands; b++ {
		f := lo * math.Pow(hi/lo, float64(b)/float64(bands))
		k := int(math.Round(f / binHz))
		if b > 0 && k <= v.edges[b-1] {
			k = v.edges[b-1] + 1
		}
		v.edges[b] = k
	}
	return v
}

// Samples — буфер, который нужно заполнить перед Frame.
func (v *Viz) Samples() []float32 { return v.samples }

// Frame пишет в out: Cols пар (min,max) как int8, затем Bands значений спектра 0..255 (−90…0 dB),
// затем пиковый уровень 0..255. Возвращает длину.
func (v *Viz) Frame(out []byte) int {
	n := v.N
	per := n / v.Cols
	o := 0
	var pk float32
	for c := 0; c < v.Cols; c++ {
		mn, mx := float32(1), float32(-1)
		for _, x := range v.samples[c*per : (c+1)*per] {
			if x < mn {
				mn = x
			}
			if x > mx {
				mx = x
			}
		}
		if abs32(mn) > pk {
			pk = abs32(mn)
		}
		if abs32(mx) > pk {
			pk = abs32(mx)
		}
		out[o], out[o+1] = byte(int8(clampF(mn)*127)), byte(int8(clampF(mx)*127))
		o += 2
	}
	for i, x := range v.samples {
		v.buf[i] = complex(float64(x)*v.win[i], 0)
	}
	fft(v.buf)
	norm := 2 / (float64(n) * 0.5)
	for b := 0; b < v.Bands; b++ {
		var m float64
		for k := v.edges[b]; k < v.edges[b+1] && k < n/2; k++ {
			if a := cmplx.Abs(v.buf[k]) * norm; a > m {
				m = a
			}
		}
		db := -120.0
		if m > 1e-9 {
			db = 20 * math.Log10(m)
		}
		// быстрый подъём, плавный спад
		if db > v.smooth[b] {
			v.smooth[b] = db
		} else {
			v.smooth[b] += (db - v.smooth[b]) * 0.35
		}
		val := (v.smooth[b] + 90) / 90 * 255
		out[o] = byte(math.Max(0, math.Min(255, val)))
		o++
	}
	out[o] = byte(math.Min(255, float64(pk)*255))
	return o + 1
}

func clampF(x float32) float32 {
	if x > 1 {
		return 1
	}
	if x < -1 {
		return -1
	}
	return x
}

// fft — БПФ по месту (радикс-2).
func fft(a []complex128) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, t := a[start+k], wk*a[start+k+size/2]
				a[start+k], a[start+k+size/2] = u+t, u-t
				wk *= w
			}
		}
	}
}

// BandHz — нижние частоты полос (для подписей в интерфейсе).
func (v *Viz) BandHz() []float64 {
	out := make([]float64, v.Bands)
	for b := range out {
		out[b] = float64(v.edges[b]) * 48000 / float64(v.N)
	}
	return out
}
