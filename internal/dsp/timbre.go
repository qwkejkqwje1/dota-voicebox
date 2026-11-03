package dsp

// Тембр — «улучшение голоса» до эффекта: срез гула, эквалайзер из пяти полос и выравнивание
// громкости (компрессор). Готовые варианты — Timbres; «свой» — любые значения полос.

// EQBand — полоса эквалайзера тембра.
type EQBand struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Hint  string  `json:"hint"`
	Freq  float64 `json:"freq"`
	Type  string  `json:"-"`
	Q     float64 `json:"-"`
}

// EQBands — полосы по порядку (значения в конфиге — в том же порядке).
var EQBands = []EQBand{
	{ID: "low", Label: "Глубина", Hint: "низ, «грудной» голос", Freq: 120, Type: "lowshelf", Q: 0.8},
	{ID: "mud", Label: "Бубнение", Hint: "меньше — чище, без «коробки»", Freq: 300, Type: "peaking", Q: 1.1},
	{ID: "mid", Label: "Середина", Hint: "напор и «нос»", Freq: 1000, Type: "peaking", Q: 1},
	{ID: "presence", Label: "Чёткость", Hint: "разборчивость в шуме боя", Freq: 3500, Type: "peaking", Q: 0.9},
	{ID: "air", Label: "Воздух", Hint: "лёгкость и блеск", Freq: 9500, Type: "highshelf", Q: 0.8},
}

// EQMax — предел усиления/ослабления полосы, дБ.
const EQMax = 12

// Timbre — готовый тембр.
type Timbre struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Emoji  string    `json:"emoji"`
	Desc   string    `json:"desc"`
	EQ     []float64 `json:"eq"`     // дБ по полосам EQBands
	Smooth float64   `json:"smooth"` // ровность громкости 0..1
}

// Timbres — готовые тембры в порядке показа ("" — без обработки, "custom" — свой — не здесь).
var Timbres = []Timbre{
	{ID: "natural", Title: "Естественный", Emoji: "🌿", Desc: "Чище и ровнее, но узнаваемо ваш: без гула и «коробки»",
		EQ: []float64{0, -2.5, 0, 1.5, 1}, Smooth: 0.3},
	{ID: "warm", Title: "Тёплый", Emoji: "☕", Desc: "Мягкий, бархатистый, без резкости",
		EQ: []float64{3, -1.5, -1, 0, -1.5}, Smooth: 0.4},
	{ID: "velvet", Title: "Бархатный", Emoji: "🎷", Desc: "Глубокий и гладкий голос с лёгким «воздухом» сверху",
		EQ: []float64{4.5, -3, -1.5, -0.5, 2.5}, Smooth: 0.55},
	{ID: "radio", Title: "Радиоведущий", Emoji: "🎙️", Desc: "Плотный эфирный голос: грудь, чёткость, ровная громкость",
		EQ: []float64{4.5, -3.5, 1, 4, 2}, Smooth: 0.75},
	{ID: "clear", Title: "Чёткий", Emoji: "🔊", Desc: "Максимально разборчиво — слышно даже в замесе",
		EQ: []float64{-1.5, -3, 1.5, 5, 3}, Smooth: 0.45},
	{ID: "deep", Title: "Глубокий", Emoji: "🗿", Desc: "Больше низа — солиднее и спокойнее",
		EQ: []float64{6.5, -1, -2, 0.5, -1}, Smooth: 0.45},
	{ID: "podcast", Title: "Подкаст", Emoji: "🎧", Desc: "Близкий, «дорогой» звук как у студийного микрофона",
		EQ: []float64{2.5, -3.5, 0, 3, 1.5}, Smooth: 0.6},
}

// FindTimbre ищет готовый тембр.
func FindTimbre(id string) (Timbre, bool) {
	for _, t := range Timbres {
		if t.ID == id {
			return t, true
		}
	}
	return Timbre{}, false
}

// TimbreSpecs — цепочка тембра: eq — дБ по полосам EQBands, smooth 0..1 — выравнивание громкости.
func TimbreSpecs(eq []float64, smooth float64) []Spec {
	s := []Spec{{Type: "highpass", Freq: 75}}
	for i, b := range EQBands {
		if i >= len(eq) {
			break
		}
		db := max(-EQMax, min(EQMax, eq[i]))
		if db > -0.05 && db < 0.05 {
			continue
		}
		s = append(s, Spec{Type: b.Type, Freq: b.Freq, Q: b.Q, DB: db})
	}
	if smooth = max(0, min(1, smooth)); smooth > 0.01 {
		s = append(s, Spec{Type: "compressor", Threshold: -16 - 16*smooth, Ratio: 1.4 + 3.6*smooth,
			AttackMS: 6, ReleaseMS: 150, MakeupDB: 1 + 7*smooth})
	}
	return append(s, Spec{Type: "limiter", Threshold: -1})
}
