package app

import (
	"fmt"
	"log"
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
)

// Выбор микрофона на лету: «следующий микрофон» по клавише, уведомление, когда выбранное
// устройство отключили, и возврат к нему, когда его снова подключат (см. Watchdog).

// micChoices — микрофоны для переключения по кругу: настоящие (без кабелей и виртуальных
// устройств) — переключаться на «другой конец» своего же кабеля бессмысленно.
func micChoices(capture []audio.Device) []string {
	var out []string
	for _, d := range capture {
		if !d.Virtual && !audio.IsCableInput(d.Name) {
			out = append(out, d.Name)
		}
	}
	return out
}

// nextMic — какой микрофон взять после текущего (cur — что работает сейчас).
func nextMic(list []string, cur string) string {
	if len(list) == 0 {
		return ""
	}
	for i, n := range list {
		if strings.EqualFold(n, cur) {
			return list[(i+1)%len(list)]
		}
	}
	return list[0]
}

// cycleMic — действие mic:next: следующий микрофон по кругу.
func (a *App) cycleMic() {
	capture, _, err := audio.Devices()
	if err != nil {
		log.Printf("Микрофоны: %v", err)
		return
	}
	list := micChoices(capture)
	cur := a.out.Status().Mic
	if len(list) < 2 {
		a.toast(Toast{Emoji: "🎙️", Text: "Микрофон один", Sub: or(cur, "не найден")})
		return
	}
	next := nextMic(list, cur)
	a.UpdateConfig(func(c *config.Config) { c.Devices.Mic = next })
	a.toast(Toast{Emoji: "🎙️", Text: next, Sub: "микрофон"})
	log.Printf("Микрофон: %s", next)
}

// noteMissing — после запуска движка: выбранное устройство не подключено → одно уведомление.
func (a *App) noteMissing() {
	st := a.out.Status()
	key := st.MicMissing + "|" + st.MonitorMissing
	a.mu.Lock()
	same := a.missingNote == key
	a.missingNote = key
	a.mu.Unlock()
	if same {
		return
	}
	if st.MicMissing != "" {
		a.toast(Toast{Emoji: "🎙️", Text: fmt.Sprintf("Не подключён: %s", st.MicMissing), Sub: fmt.Sprintf("пока работает %s", st.Mic)})
	}
	if st.MonitorMissing != "" {
		a.toast(Toast{Emoji: "🎧", Text: fmt.Sprintf("Не подключены: %s", st.MonitorMissing), Sub: fmt.Sprintf("звук идёт в %s", st.Monitor)})
	}
}

// missingBack — выбранный микрофон или наушники снова подключили (вызывает сторож).
func missingBack(st audio.Status, capture, playback []audio.Device) bool {
	return st.MicMissing != "" && audio.HasDevice(capture, st.MicMissing) ||
		st.MonitorMissing != "" && audio.HasDevice(playback, st.MonitorMissing)
}
