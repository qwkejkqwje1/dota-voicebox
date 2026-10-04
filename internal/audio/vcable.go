package audio

import (
	"regexp"
	"strings"
)

// Распознавание виртуальных кабелей любых производителей:
//   VB-Audio Cable / A+B / Hi-Fi:  "CABLE Input (VB-Audio Virtual Cable)" ↔ "CABLE Output (…)"
//   Voicemeeter:                   "Voicemeeter Input" ↔ "Voicemeeter Out B1", AUX ↔ B2, VAIO3 ↔ B3
//   Virtual Audio Cable (VAC):     "Line 1 (Virtual Audio Cable)" — одинаковое имя у обоих концов
//   прочие:                        всё, где есть «virtual», «cable», «vac».
// Выбор пользователя всегда главнее: любое устройство можно назначить кабелем вручную.

var virtualRe = regexp.MustCompile(`(?i)vb-audio|voicemeeter|vaio|virtual|\bcable\b|\bvac\b|^line ?\d+\s*(\(|$)`)

// IsVirtual — похоже ли устройство на виртуальный кабель.
func IsVirtual(name string) bool { return virtualRe.MatchString(strings.TrimSpace(name)) }

// cableScore — насколько устройство вывода подходит как «вход» кабеля (0 — не подходит).
func cableScore(name string) int {
	n := strings.ToLower(name)
	switch {
	case !IsVirtual(name):
		return 0
	case strings.Contains(n, "output") || strings.Contains(n, " out "):
		return 1 // устройство вывода с «output» в имени — странно, но лучше, чем ничего
	case strings.Contains(n, "cable input") && !strings.Contains(n, "cable-"):
		return 100
	case strings.Contains(n, "cable") && strings.Contains(n, "input"):
		return 90
	case strings.Contains(n, "voicemeeter input"), strings.Contains(n, "voicemeeter vaio"):
		return 80
	case strings.Contains(n, "voicemeeter aux"):
		return 75
	case regexp.MustCompile(`^line ?1\b`).MatchString(n):
		return 72
	case regexp.MustCompile(`^line ?\d+`).MatchString(n):
		return 70
	case strings.Contains(n, "voicemeeter"):
		return 60
	}
	return 50
}

// FindCable ищет виртуальный кабель среди устройств вывода.
func FindCable(playback []Device) string {
	best, bs := "", 0
	for _, d := range playback {
		if s := cableScore(d.Name); s > bs {
			best, bs = d.Name, s
		}
	}
	return best
}

// IsCableInput — является ли устройство ввода выходом виртуального кабеля
// (его нельзя выбирать как свой микрофон — будет петля).
func IsCableInput(name string) bool { return IsVirtual(name) }

func base(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	if i := strings.Index(n, " ("); i > 0 {
		return n[:i]
	}
	return n
}
func suffix(n string) string {
	n = strings.ToLower(n)
	if i := strings.Index(n, " ("); i > 0 {
		return n[i:]
	}
	return ""
}

// CablePair — «другой конец» кабеля: устройство записи, на которое приходит звук из play.
// Возвращает "" если подходящего нет.
func CablePair(play string, capture []Device) string {
	p := strings.ToLower(play)
	pb := base(play)
	var want []string
	switch {
	case strings.Contains(p, "voicemeeter aux"):
		want = []string{"voicemeeter out b2", "voicemeeter aux output"}
	case strings.Contains(p, "voicemeeter vaio3"):
		want = []string{"voicemeeter out b3", "voicemeeter vaio3 output"}
	case strings.Contains(p, "voicemeeter"):
		want = []string{"voicemeeter out b1", "voicemeeter output"}
	}
	best, bs := "", 0
	for _, d := range capture {
		n := strings.ToLower(d.Name)
		s := 0
		switch {
		case n == p:
			s = 100 // VAC: «Line 1 (Virtual Audio Cable)» с обеих сторон
		case strings.Replace(p, "input", "output", 1) == n:
			s = 95
		case base(d.Name) == pb && suffix(d.Name) == suffix(play):
			s = 90
		case strings.Replace(pb, "input", "output", 1) == base(d.Name):
			s = 85
		case base(d.Name) == pb:
			s = 80
		}
		for i, w := range want {
			if strings.Contains(n, w) && 70-i > s {
				s = 70 - i
			}
		}
		if s == 0 && IsVirtual(d.Name) && suffix(d.Name) != "" && suffix(d.Name) == suffix(play) && strings.Contains(n, "output") {
			s = 40 // тот же драйвер, «… Output»
		}
		if s > bs {
			best, bs = d.Name, s
		}
	}
	return best
}

// matchDevice — индекс устройства по имени: точное совпадение важнее подстроки
// (иначе «Line 1» совпало бы с «Line 10»).
func matchDevice(names []string, sub string) int {
	s := strings.ToLower(strings.TrimSpace(sub))
	if s == "" {
		return -1
	}
	for i, n := range names {
		if strings.ToLower(n) == s {
			return i
		}
	}
	for i, n := range names {
		if base(n) == s || base(n) == base(sub) && suffix(sub) == "" {
			return i
		}
	}
	for i, n := range names {
		if strings.Contains(strings.ToLower(n), s) {
			return i
		}
	}
	return -1
}
