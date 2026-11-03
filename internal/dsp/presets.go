package dsp

// Каталог встроенных пресетов голоса. У пресета есть «ручки» (Knobs) — понятные настройки
// в интерфейсе; их значения хранятся в config.json → "preset_params" и превращаются в цепочку
// эффектов функцией build. Любой пресет можно переопределить своим описанием в "voice_presets".

// Knob — настройка пресета.
type Knob struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // range | toggle | choice
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Def     float64  `json:"def"`
	Unit    string   `json:"unit,omitempty"`
	Options []Option `json:"options,omitempty"`
}

// Option — вариант для Kind = choice.
type Option struct {
	Value float64 `json:"value"`
	Label string  `json:"label"`
}

// Preset — встроенный пресет голоса.
type Preset struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Emoji string `json:"emoji"`
	Desc  string `json:"desc"`
	Group string `json:"group"`
	Knobs []Knob `json:"knobs,omitempty"`
	build func(p params) []Spec
}

type params map[string]float64

// Values дополняет значения ручек значениями по умолчанию и обрезает их по диапазону.
func (p Preset) Values(v map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for _, k := range p.Knobs {
		x, ok := v[k.ID]
		if !ok {
			x = k.Def
		}
		switch k.Kind {
		case "toggle":
			if x != 0 {
				x = 1
			}
		case "choice":
			valid := false
			for _, o := range k.Options {
				valid = valid || o.Value == x
			}
			if !valid {
				x = k.Def
			}
		default:
			x = max(k.Min, min(k.Max, x))
		}
		out[k.ID] = x
	}
	return out
}

// Specs — цепочка эффектов для значений ручек (недостающие — по умолчанию).
func (p Preset) Specs(v map[string]float64) []Spec {
	if p.build == nil {
		return nil
	}
	return p.build(p.Values(v))
}

// FindPreset ищет встроенный пресет.
func FindPreset(id string) (Preset, bool) {
	for _, p := range Catalog {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

const (
	GroupBase  = "Основные"
	GroupRadio = "Рация и связь"
	GroupChars = "Персонажи"
	GroupMusic = "Музыка"
	GroupSpace = "Пространство"
	GroupMemes = "Мемы"
)

func pct(id, label string, def float64) Knob {
	return Knob{ID: id, Label: label, Kind: "range", Min: 0, Max: 100, Step: 1, Def: def, Unit: "%"}
}

func toggle(id, label string, def bool) Knob {
	d := 0.0
	if def {
		d = 1
	}
	return Knob{ID: id, Label: label, Kind: "toggle", Def: d}
}

func semis(def, lo, hi float64) Knob {
	return Knob{ID: "pitch", Label: "Высота голоса", Kind: "range", Min: lo, Max: hi, Step: 1, Def: def, Unit: "пт"}
}

var beepModes = []string{"", "beep", "double", "chirp", "quindar"}

func beepKnob(def float64) Knob {
	return Knob{ID: "beep", Label: "Сигнал в конце фразы", Kind: "choice", Def: def, Options: []Option{
		{0, "Нет"}, {1, "Бип"}, {2, "Двойной"}, {3, "Чирп"}, {4, "Квиндар (NASA)"},
	}}
}

// radioKnobs — общие ручки всех раций; различаются только значения по умолчанию.
func radioKnobs(noise, drive, narrow, interf, digital float64, squelch bool, beep float64, tpt bool) []Knob {
	return []Knob{
		pct("noise", "Шум эфира", noise),
		pct("drive", "Перегруз", drive),
		pct("narrow", "Узость полосы", narrow),
		pct("interf", "Помехи", interf),
		pct("digital", "Цифровая грязь", digital),
		toggle("squelch", "Щелчок в начале и «пшш» в конце", squelch),
		beepKnob(beep),
		toggle("tpt", "Сигнал в начале передачи", tpt),
	}
}

func buildRadio(p params) []Spec {
	n := p["narrow"]
	s := Spec{Type: "radio", LowHz: 250 + 3*n, HighHz: 3800 - 16*n, Drive: p["drive"] / 100,
		Interference: p["interf"] / 100, Digital: p["digital"] / 100,
		NoSquelch: p["squelch"] == 0, TPT: p["tpt"] == 1}
	if v := p["noise"]; v > 0 {
		s.NoiseDB = -58 + 0.32*v
	}
	if b := int(p["beep"]); b > 0 && b < len(beepModes) {
		s.Beep = beepModes[b]
	}
	return []Spec{s}
}

var noteNames = []string{"До", "До♯", "Ре", "Ре♯", "Ми", "Фа", "Фа♯", "Соль", "Соль♯", "Ля", "Ля♯", "Си"}
var tuneScales = []string{"chromatic", "major", "minor"}

// tuneKnobs — ручки автотюна; speed — плавность по умолчанию (0 — мгновенно).
func tuneKnobs(speed float64, withSpeed bool) []Knob {
	keys := make([]Option, len(noteNames))
	for i, n := range noteNames {
		keys[i] = Option{float64(i), n}
	}
	k := []Knob{
		{ID: "scale", Label: "Ноты", Kind: "choice", Def: 0, Options: []Option{{0, "Все (проще всего)"}, {1, "Мажор"}, {2, "Минор"}}},
		{ID: "key", Label: "Тональность (для мажора и минора)", Kind: "choice", Def: 0, Options: keys},
	}
	if withSpeed {
		k = append(k, pct("speed", "Плавность подстройки", speed))
	}
	return append(k, semis(0, -12, 12), pct("mix", "Сила", 100))
}

func buildTune(p params) []Spec {
	sc := tuneScales[0]
	if i := int(p["scale"]); i > 0 && i < len(tuneScales) {
		sc = tuneScales[i]
	}
	return []Spec{
		{Type: "highpass", Freq: 70},
		{Type: "autotune", Key: int(p["key"]), Scale: sc, Speed: p["speed"] / 100, Semitones: p["pitch"], Mix: max(p["mix"]/100, 0.001)},
		{Type: "compressor", Threshold: -22, Ratio: 2.5, AttackMS: 5, ReleaseMS: 120, MakeupDB: 3},
		{Type: "limiter", Threshold: -1},
	}
}

// Catalog — встроенные пресеты в порядке показа.
var Catalog = []Preset{
	{ID: "clean", Title: "Чистый", Emoji: "🎙️", Group: GroupBase,
		Desc:  "Ваш голос без обработки",
		build: func(params) []Spec { return []Spec{} }},
	{ID: "studio", Title: "Студийный", Emoji: "✨", Group: GroupBase,
		Desc:  "Плотный и разборчивый голос, как у ведущего: срез гула, теплота, яркость, компрессия",
		Knobs: []Knob{{ID: "presence", Label: "Яркость", Kind: "range", Min: 0, Max: 6, Step: 0.5, Def: 3, Unit: "дБ"}, pct("comp", "Компрессия", 50)},
		build: func(p params) []Spec {
			c := p["comp"] / 100
			return []Spec{
				{Type: "highpass", Freq: 80},
				{Type: "peaking", Freq: 320, Q: 1, DB: -2},
				{Type: "lowshelf", Freq: 120, DB: 2},
				{Type: "peaking", Freq: 4500, Q: 0.8, DB: p["presence"]},
				{Type: "compressor", Threshold: -24, Ratio: 1.5 + 4.5*c, AttackMS: 5, ReleaseMS: 120, MakeupDB: 2 + 6*c},
				{Type: "limiter", Threshold: -1},
			}
		}},

	{ID: "radio", Title: "Рация", Emoji: "📻", Group: GroupRadio,
		Desc:  "Портативная рация: узкая полоса, перегруз, шипение эфира, щелчок в начале и «пшш» в конце фразы",
		Knobs: radioKnobs(45, 40, 40, 0, 0, true, 0, false), build: buildRadio},
	{ID: "radio_military", Title: "Военная рация", Emoji: "🪖", Group: GroupRadio,
		Desc:  "Тактическая связь: жёсткая компрессия, цифровой оттенок, сигнал в начале и двойной бип в конце",
		Knobs: radioKnobs(30, 55, 60, 5, 35, true, 2, true), build: buildRadio},
	{ID: "radio_broken", Title: "Плохая связь", Emoji: "📡", Group: GroupRadio,
		Desc:  "Рация на краю зоны приёма: треск, выпадения, замирания и сильный шум",
		Knobs: radioKnobs(70, 65, 65, 75, 10, true, 0, false), build: buildRadio},
	{ID: "space", Title: "ЦУП", Emoji: "🚀", Group: GroupRadio,
		Desc:  "Связь с орбитой: тоны Квиндара в начале и конце фразы, как в записях NASA",
		Knobs: radioKnobs(40, 25, 45, 5, 0, false, 4, false), build: buildRadio},
	{ID: "telephone", Title: "Телефон", Emoji: "☎️", Group: GroupRadio,
		Desc:  "Телефонная линия: полоса 300–3400 Гц и 8-битный кодек",
		Knobs: []Knob{pct("noise", "Шум линии", 30)},
		build: func(p params) []Spec {
			s := []Spec{
				{Type: "highpass", Freq: 300, Q: 0.5412}, {Type: "highpass", Freq: 300, Q: 1.3066},
				{Type: "lowpass", Freq: 3400, Q: 0.5412}, {Type: "lowpass", Freq: 3400, Q: 1.3066},
				{Type: "compressor", Threshold: -26, Ratio: 4, AttackMS: 3, ReleaseMS: 100, MakeupDB: 8},
				{Type: "drive", Amount: 0.15, Mode: "soft"},
				{Type: "mulaw", Hold: 6},
			}
			if v := p["noise"]; v > 0 {
				s = append(s, Spec{Type: "noise", DB: -66 + 0.3*v})
			}
			return append(s,
				Spec{Type: "lowpass", Freq: 3400, Q: 0.5412}, Spec{Type: "lowpass", Freq: 3400, Q: 1.3066},
				Spec{Type: "peaking", Freq: 1500, Q: 1, DB: 3},
				Spec{Type: "limiter", Threshold: -1})
		}},
	{ID: "megaphone", Title: "Мегафон", Emoji: "📢", Group: GroupRadio,
		Desc:  "Громкоговоритель: рупор, перегруз и эхо улицы",
		Knobs: []Knob{pct("drive", "Перегруз", 60), pct("echo", "Эхо улицы", 35)},
		build: func(p params) []Spec {
			s := []Spec{
				{Type: "gate", Threshold: -50},
				{Type: "highpass", Freq: 450, Q: 0.9}, {Type: "highpass", Freq: 450, Q: 0.9},
				{Type: "lowpass", Freq: 4000, Q: 0.9}, {Type: "lowpass", Freq: 4000, Q: 0.9},
				{Type: "peaking", Freq: 1400, Q: 1.2, DB: 7},
				{Type: "peaking", Freq: 2800, Q: 2, DB: 4},
				{Type: "gain", DB: 12},
				{Type: "drive", Amount: 0.2 + 0.8*p["drive"]/100, Mode: "soft"},
				{Type: "comb", Freq: 900, Feedback: 0.25, Mix: 0.3},
			}
			if e := p["echo"]; e > 0 {
				s = append(s, Spec{Type: "echo", DelayMS: 110, Feedback: 0.25, Mix: e / 100 * 0.5, Freq: 2500})
			}
			return append(s, Spec{Type: "limiter", Threshold: -1})
		}},

	{ID: "robot", Title: "Робот", Emoji: "🤖", Group: GroupChars,
		Desc:  "Металлический голос машины: кольцевая модуляция и резонатор",
		Knobs: []Knob{{ID: "tone", Label: "Тон", Kind: "range", Min: 30, Max: 200, Step: 5, Def: 60, Unit: "Гц"}, pct("metal", "Металл", 50)},
		build: func(p params) []Spec {
			return []Spec{
				{Type: "gate", Threshold: -50},
				{Type: "ringmod", Freq: p["tone"], Mix: 0.9},
				{Type: "comb", Freq: p["tone"] * 2, Feedback: 0.85 * p["metal"] / 100, Mix: 0.6},
				{Type: "bitcrush", Bits: 8, Down: 2},
				{Type: "highpass", Freq: 100},
				{Type: "gain", DB: 3},
				{Type: "limiter", Threshold: -1},
			}
		}},
	{ID: "demon", Title: "Демон", Emoji: "👹", Group: GroupChars,
		Desc:  "Низкий рычащий голос из преисподней",
		Knobs: []Knob{semis(-8, -12, -4), pct("echo", "Эхо", 30)},
		build: func(p params) []Spec {
			s := []Spec{
				{Type: "gate", Threshold: -48},
				{Type: "pitch", Semitones: p["pitch"], WindowMS: 55},
				{Type: "lowshelf", Freq: 180, DB: 5},
				{Type: "drive", Amount: 0.3, Mode: "soft"},
				{Type: "highpass", Freq: 45},
			}
			if e := p["echo"]; e > 0 {
				s = append(s, Spec{Type: "reverb", Room: 0.6, Damp: 0.5, Wet: e / 100 * 0.5, Dry: 1})
			}
			return append(s, Spec{Type: "limiter", Threshold: -1})
		}},
	{ID: "deep", Title: "Низкий голос", Emoji: "🗿", Group: GroupChars,
		Desc:  "Голос ниже и солиднее, без лишних эффектов",
		Knobs: []Knob{semis(-4, -8, -1)},
		build: func(p params) []Spec {
			return []Spec{
				{Type: "pitch", Semitones: p["pitch"], WindowMS: 50},
				{Type: "lowshelf", Freq: 150, DB: 3},
				{Type: "compressor", Threshold: -22, Ratio: 3, AttackMS: 5, ReleaseMS: 120, MakeupDB: 3},
				{Type: "limiter", Threshold: -1},
			}
		}},
	{ID: "chipmunk", Title: "Бурундук", Emoji: "🐿️", Group: GroupChars,
		Desc:  "Писклявый мультяшный голос",
		Knobs: []Knob{semis(7, 3, 12)},
		build: func(p params) []Spec {
			return []Spec{
				{Type: "pitch", Semitones: p["pitch"], WindowMS: 30},
				{Type: "highpass", Freq: 120},
				{Type: "limiter", Threshold: -1},
			}
		}},
	{ID: "alien", Title: "Пришелец", Emoji: "👽", Group: GroupChars,
		Desc:  "Нечеловеческий плавающий голос с металлическим отливом",
		Knobs: []Knob{semis(3, -6, 8), pct("wobble", "Вибрация", 50)},
		build: func(p params) []Spec {
			s := []Spec{{Type: "gate", Threshold: -50}}
			if p["pitch"] != 0 {
				s = append(s, Spec{Type: "pitch", Semitones: p["pitch"], WindowMS: 35})
			}
			if w := p["wobble"]; w > 0 {
				s = append(s, Spec{Type: "vibrato", Depth: 0.4 + 2.6*w/100, RateHz: 6})
			}
			return append(s,
				Spec{Type: "ringmod", Freq: 420, Mix: 0.25},
				Spec{Type: "flanger", Depth: 2, RateHz: 0.3, Feedback: 0.5, Mix: 0.5},
				Spec{Type: "reverb", Room: 0.4, Damp: 0.3, Wet: 0.15, Dry: 1},
				Spec{Type: "limiter", Threshold: -1})
		}},

	{ID: "autotune", Title: "Автотюн", Emoji: "🎤", Group: GroupMusic,
		Desc:  "Подтягивает голос к нотам, как у поп-певцов: пойте — и попадайте",
		Knobs: tuneKnobs(25, true),
		build: buildTune},
	{ID: "hardtune", Title: "Робо-тюн", Emoji: "🎶", Group: GroupMusic,
		Desc:  "Жёсткий автотюн с «ступеньками» между нотами — как в хитах 2000-х",
		Knobs: tuneKnobs(0, false),
		build: func(p params) []Spec {
			return append(buildTune(p), Spec{Type: "chorus", Depth: 2, RateHz: 0.6, Mix: 0.25})
		}},

	{ID: "announcer", Title: "Диктор стадиона", Emoji: "🏟️", Group: GroupSpace,
		Desc:  "Объявление на весь стадион: плотный голос и раскатистое эхо",
		Knobs: []Knob{pct("echo", "Эхо стадиона", 50)},
		build: func(p params) []Spec {
			e := p["echo"] / 100
			s := []Spec{
				{Type: "highpass", Freq: 100},
				{Type: "compressor", Threshold: -24, Ratio: 4, AttackMS: 5, ReleaseMS: 150, MakeupDB: 6},
				{Type: "peaking", Freq: 3000, Q: 1, DB: 3},
			}
			if e > 0 {
				s = append(s,
					Spec{Type: "echo", DelayMS: 380, Feedback: 0.3, Mix: 0.35 * e, Freq: 3000},
					Spec{Type: "reverb", Room: 0.85, Damp: 0.4, Wet: 0.4 * e, Dry: 1, PreDelayMS: 40})
			}
			return append(s, Spec{Type: "limiter", Threshold: -1})
		}},
	{ID: "cave", Title: "Пещера", Emoji: "🕳️", Group: GroupSpace,
		Desc:  "Огромное гулкое пространство с отражениями",
		Knobs: []Knob{pct("size", "Размер", 70), pct("echo", "Эхо", 50)},
		build: func(p params) []Spec {
			e := p["echo"] / 100
			return []Spec{
				{Type: "reverb", Room: 0.5 + 0.5*p["size"]/100, Damp: 0.25, Wet: 0.15 + 0.45*e, Dry: 1, PreDelayMS: 20},
				{Type: "echo", DelayMS: 320, Feedback: 0.35, Mix: 0.3 * e, Freq: 2500},
				{Type: "limiter", Threshold: -1},
			}
		}},
	{ID: "underwater", Title: "Под водой", Emoji: "🌊", Group: GroupSpace,
		Desc:  "Глухой булькающий звук, как из-под воды",
		Knobs: []Knob{pct("depth", "Глубина", 60)},
		build: func(p params) []Spec {
			f := 1400 - 11*p["depth"]
			return []Spec{
				{Type: "lowpass", Freq: f, Q: 0.9}, {Type: "lowpass", Freq: f, Q: 0.9},
				{Type: "vibrato", Depth: 3, RateHz: 0.9},
				{Type: "chorus", Depth: 6, RateHz: 0.4, Mix: 0.5},
				{Type: "reverb", Room: 0.4, Damp: 0.9, Wet: 0.25, Dry: 1},
				{Type: "gain", DB: 4},
				{Type: "limiter", Threshold: -1},
			}
		}},

	{ID: "dead_inside", Title: "Дед инсайд", Emoji: "💀", Group: GroupMemes,
		Desc:  "Передавленный микрофон: перегруз и раздутый бас",
		Knobs: []Knob{pct("drive", "Перегруз", 90)},
		build: func(p params) []Spec {
			return []Spec{
				{Type: "gate", Threshold: -48, HangMS: 150},
				{Type: "highpass", Freq: 90},
				{Type: "lowshelf", Freq: 180, DB: 9},
				{Type: "gain", DB: 22},
				{Type: "drive", Amount: max(0.05, p["drive"]/100), Mode: "soft"},
				{Type: "bitcrush", Bits: 10, Down: 2},
				{Type: "peaking", Freq: 2500, Q: 1, DB: 6},
				{Type: "drive", Amount: 0.3, Mode: "hard"},
				{Type: "lowpass", Freq: 9000},
				{Type: "gain", DB: -4},
				{Type: "limiter", Threshold: -1},
			}
		}},
	{ID: "dead_inside_max", Title: "Ушная боль", Emoji: "💢", Group: GroupMemes,
		Desc: "Ломаный цифровой перегруз — осторожно, громко",
		build: func(params) []Spec {
			return []Spec{
				{Type: "gate", Threshold: -45, HangMS: 150},
				{Type: "highpass", Freq: 70},
				{Type: "lowshelf", Freq: 150, DB: 12},
				{Type: "gain", DB: 34},
				{Type: "drive", Amount: 1, Mode: "hard"},
				{Type: "bitcrush", Bits: 6, Down: 3},
				{Type: "drive", Amount: 0.6, Mode: "fold"},
				{Type: "peaking", Freq: 1800, Q: 0.8, DB: 8},
				{Type: "lowpass", Freq: 7000},
				{Type: "gain", DB: -6},
				{Type: "limiter", Threshold: -1},
			}
		}},
}

// BuiltinPresets — цепочки встроенных пресетов со значениями по умолчанию.
var BuiltinPresets = map[string][]Spec{}

// PresetOrder — порядок переключения по "preset:next".
var PresetOrder []string

func init() {
	for _, p := range Catalog {
		BuiltinPresets[p.ID] = p.Specs(nil)
		PresetOrder = append(PresetOrder, p.ID)
	}
}
