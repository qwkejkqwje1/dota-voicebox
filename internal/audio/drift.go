package audio

import (
	"sync/atomic"
)

// spsc — кольцевой буфер «один писатель / один читатель» без блокировок.
type spsc struct {
	buf  []float32
	mask uint64
	w, r atomic.Uint64
}

func newSPSC(minSize int) *spsc {
	n := 1
	for n < minSize {
		n <<= 1
	}
	return &spsc{buf: make([]float32, n), mask: uint64(n - 1)}
}

// Write пишет сэмплы; при переполнении лишнее отбрасывается (читатель отстал).
func (q *spsc) Write(p []float32) {
	w, r := q.w.Load(), q.r.Load()
	free := uint64(len(q.buf)) - (w - r)
	if uint64(len(p)) > free {
		p = p[:free]
	}
	for _, x := range p {
		q.buf[w&q.mask] = x
		w++
	}
	q.w.Store(w)
}

func (q *spsc) Avail() int { return int(q.w.Load() - q.r.Load()) }

// Skip выбрасывает n самых старых сэмплов.
func (q *spsc) Skip(n int) { q.r.Add(uint64(n)) }

// drift — чтение потока с чужого устройства (у которого свои часы) с адаптивным
// ресемплингом: скорость чтения подстраивается под заполнение буфера, поэтому
// буфер не опустошается и не переполняется даже через часы работы.
type drift struct {
	q       *spsc
	target  float64 // желаемое заполнение, сэмплы
	fill    float64 // сглаженное заполнение
	ratio   float64
	pos     float64
	a, b    float32 // два последних входных сэмпла для интерполяции
	primed  bool
	Under   atomic.Uint64 // счётчик опустошений (xruns)
	maxStep float64
}

func newDrift(targetSamples int) *drift {
	return &drift{q: newSPSC(targetSamples * 16), target: float64(targetSamples), fill: float64(targetSamples), ratio: 1, maxStep: 0.005}
}

func (d *drift) Write(p []float32) { d.q.Write(p) }

// Ratio — текущий коэффициент (1.0 = часы совпадают).
func (d *drift) Ratio() float64 { return d.ratio }

// Pull заполняет out ровно len(out) сэмплами (тишина, если данных нет).
func (d *drift) Pull(out []float32) {
	avail := d.q.Avail()
	if !d.primed {
		if float64(avail) < d.target {
			clear(out)
			return
		}
		d.primed, d.pos, d.fill = true, 1, float64(avail)
	}
	// после долгой паузы/рывка накопилось слишком много — сбрасываем до цели
	if float64(avail) > d.target*6 {
		d.q.Skip(avail - int(d.target))
		avail = int(d.target)
		d.fill = d.target
	}
	d.fill += (float64(avail) - d.fill) * 0.02
	e := (d.fill - d.target) / d.target
	r := 1 + e*0.03
	if r > 1+d.maxStep {
		r = 1 + d.maxStep
	} else if r < 1-d.maxStep {
		r = 1 - d.maxStep
	}
	d.ratio = r

	q := d.q
	rIdx, wIdx := q.r.Load(), q.w.Load()
	for i := range out {
		for d.pos >= 1 {
			if rIdx == wIdx {
				q.r.Store(rIdx)
				clear(out[i:])
				d.primed = false
				d.Under.Add(1)
				return
			}
			d.a, d.b = d.b, q.buf[rIdx&q.mask]
			rIdx++
			d.pos--
		}
		out[i] = d.a + (d.b-d.a)*float32(d.pos)
		d.pos += r
	}
	q.r.Store(rIdx)
}
