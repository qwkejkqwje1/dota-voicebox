package rnnoise

// Stream — шумоподавление для аудиопотока кусками любой длины. Копит вход до кадра в 10 мс,
// поэтому к задержке сети (20 мс) добавляется до 10 мс, если звуковая карта отдаёт куски не по 10 мс.
// Включение и выключение — с плавным переходом 10 мс, без щелчков. Без выделения памяти в Process.
type Stream struct {
	st *State

	acc  [FrameSize]float32 // накопленный вход текущего кадра
	nacc int
	hist [Delay + FrameSize]float32 // вход за последние кадры — «сухой» сигнал, выровненный с выходом сети

	out   [4 * 4096]float32 // готовый выход (кольцо)
	r, w  int
	frame [FrameSize]float32
	on    bool
	fade  int // отсчётов до конца перехода вкл/выкл
	vad   float32
}

const outMask = 4*4096 - 1

// NewStream создаёт поток (загружает модель при первом вызове).
func NewStream() (*Stream, error) {
	st, err := NewState()
	if err != nil {
		return nil, err
	}
	return &Stream{st: st}, nil
}

// Close освобождает память.
func (s *Stream) Close() { s.st.Close() }

// VAD — вероятность речи в последнем кадре (0..1).
func (s *Stream) VAD() float32 { return s.vad }

func (s *Stream) buffered() int { return (s.w - s.r) & outMask }

// Process чистит буфер на месте. strength 0..1: 1 — полное подавление, меньше — часть исходного
// сигнала остаётся (шум тише максимум на 20·lg(1/(1−strength)) дБ, голос естественнее).
// on=false — сигнал проходит без изменений и без задержки.
func (s *Stream) Process(buf []float32, on bool, strength float32) {
	if on != s.on {
		s.on = on
		s.fade = fadeLen
		if on {
			s.reset(len(buf))
		}
	}
	if !s.on && s.fade == 0 {
		return
	}
	if strength < 0 {
		strength = 0
	} else if strength > 1 {
		strength = 1
	}
	for _, x := range buf { // сначала весь кусок в сеть: при кусках по 10 мс выход готов сразу
		s.acc[s.nacc] = x
		s.nacc++
		if s.nacc == FrameSize {
			s.nacc = 0
			s.runFrame(strength)
		}
	}
	for i, x := range buf {
		var y float32
		if s.buffered() > 0 { // иначе (кусок не по 10 мс сразу после включения) — тишина, очередь подрастёт
			y = s.out[s.r]
			s.r = (s.r + 1) & outMask
		}
		if s.fade > 0 { // переход: смешиваем исходный и очищенный сигнал
			g := float32(s.fade) / fadeLen
			if s.on {
				g = 1 - g
			}
			y = y*g + x*(1-g)
			s.fade--
		}
		buf[i] = y
	}
}

const fadeLen = FrameSize

// reset — новый старт после включения: очистить очереди. Если звуковая карта отдаёт куски,
// кратные 10 мс, лишней задержки нет; иначе в очереди сразу кадр тишины, чтобы выход не прерывался.
func (s *Stream) reset(n int) {
	s.nacc, s.r, s.w = 0, 0, 0
	for i := range s.hist {
		s.hist[i] = 0
	}
	if n%FrameSize != 0 {
		for i := 0; i < FrameSize; i++ {
			s.out[s.w] = 0
			s.w = (s.w + 1) & outMask
		}
	}
}

func (s *Stream) runFrame(strength float32) {
	// история входа: сдвиг на кадр, новый кадр в конец
	copy(s.hist[:], s.hist[FrameSize:])
	copy(s.hist[Delay:], s.acc[:])
	copy(s.frame[:], s.acc[:])
	s.vad = s.st.Frame(s.frame[:])
	dry := s.hist[:FrameSize] // вход Delay отсчётов назад — совпадает по времени с выходом сети
	k := 1 - strength
	for i := 0; i < FrameSize; i++ {
		y := s.frame[i]
		if k > 0 {
			y = y*strength + dry[i]*k
		}
		if s.buffered() >= outMask { // переполнение (не должно случаться) — выбрасываем старое
			s.r = (s.r + 1) & outMask
		}
		s.out[s.w] = y
		s.w = (s.w + 1) & outMask
	}
}
