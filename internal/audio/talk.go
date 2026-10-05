package audio

// Режим «говорю только по кнопке»: микрофон идёт в эфир, только пока открыт.
// Плавное открытие (3 мс) и закрытие (10 мс) — без щелчков. Звуки и источники не затрагиваются.

// SetTalkGate включает/выключает режим.
func (e *Engine) SetTalkGate(on bool) { e.talkGate.Store(on) }

// SetTalk открывает/закрывает микрофон в режиме «по кнопке».
func (e *Engine) SetTalk(open bool) { e.talkOpen.Store(open) }

// TalkOpen — идёт ли сейчас голос в эфир (с учётом режима).
func (e *Engine) TalkOpen() bool { return !e.talkGate.Load() || e.talkOpen.Load() }

func (e *Engine) talk(b []float32) {
	target := float32(1)
	if e.talkGate.Load() && !e.talkOpen.Load() {
		target = 0
	}
	if target == 1 && e.talkGain >= 1 {
		return
	}
	const up, down = 1.0 / 144, 1.0 / 480
	for i := range b {
		if target > e.talkGain {
			e.talkGain = min(1, e.talkGain+up)
		} else if target < e.talkGain {
			e.talkGain = max(0, e.talkGain-down)
		}
		b[i] *= e.talkGain
	}
}
