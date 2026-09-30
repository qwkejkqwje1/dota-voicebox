package dsp

// BuiltinPresets — встроенные пресеты голоса. Любой можно переопределить
// или добавить свой в config.json → "voice_presets".
var BuiltinPresets = map[string][]Spec{
	// Чистый голос.
	"clean": {},

	// «Дед инсайд»: передавленный, перегруженный микрофон с раздутым басом.
	// Гейт первым, чтобы не усиливать фоновый шум на +20 дБ.
	"dead_inside": {
		{Type: "gate", Threshold: -48, HangMS: 150},
		{Type: "highpass", Freq: 90},
		{Type: "lowshelf", Freq: 180, DB: 9},
		{Type: "gain", DB: 22},
		{Type: "drive", Amount: 0.9, Mode: "soft"},
		{Type: "bitcrush", Bits: 10, Down: 2},
		{Type: "peaking", Freq: 2500, Q: 1, DB: 6},
		{Type: "drive", Amount: 0.3, Mode: "hard"},
		{Type: "lowpass", Freq: 9000},
		{Type: "gain", DB: -4},
		{Type: "limiter", Threshold: -1},
	},

	// «Дед инсайд MAX»: ушная боль, ломаный цифровой перегруз.
	"dead_inside_max": {
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
	},

	// Рация: узкая полоса 350–3200 Гц, лёгкий перегруз, компрессия,
	// щелчок в начале фразы, шипение и «хвост» шума в конце.
	"radio": {
		{Type: "squelch", Threshold: -46, HangMS: 220, ClickDB: -10, StaticDB: -38, TailMS: 140, TailDB: -16},
		{Type: "highpass", Freq: 350, Q: 0.9},
		{Type: "highpass", Freq: 350, Q: 0.9},
		{Type: "lowpass", Freq: 3200, Q: 0.9},
		{Type: "lowpass", Freq: 3200, Q: 0.9},
		{Type: "peaking", Freq: 1800, Q: 1.2, DB: 5},
		{Type: "gain", DB: 10},
		{Type: "drive", Amount: 0.4, Mode: "soft"},
		{Type: "compressor", Threshold: -18, Ratio: 6, AttackMS: 3, ReleaseMS: 80, MakeupDB: 3},
		{Type: "bitcrush", Bits: 12, Down: 1},
		{Type: "limiter", Threshold: -1},
	},

	// Рация с плохим приёмом: треск, выпадения, цифровая грязь.
	"radio_broken": {
		{Type: "squelch", Threshold: -46, HangMS: 220, ClickDB: -8, StaticDB: -30, TailMS: 200, TailDB: -12},
		{Type: "highpass", Freq: 450, Q: 0.9},
		{Type: "highpass", Freq: 450, Q: 0.9},
		{Type: "lowpass", Freq: 2600, Q: 0.9},
		{Type: "lowpass", Freq: 2600, Q: 0.9},
		{Type: "gain", DB: 14},
		{Type: "drive", Amount: 0.7, Mode: "hard"},
		{Type: "bitcrush", Bits: 7, Down: 3},
		{Type: "dropout", Chance: 0.03, LenMS: 70},
		{Type: "noise", DB: -40, Kind: "crackle"},
		{Type: "compressor", Threshold: -16, Ratio: 8, AttackMS: 2, ReleaseMS: 60},
		{Type: "limiter", Threshold: -1},
	},

	// Мегафон / громкоговоритель.
	"megaphone": {
		{Type: "gate", Threshold: -50},
		{Type: "highpass", Freq: 500},
		{Type: "lowpass", Freq: 4500},
		{Type: "peaking", Freq: 1500, Q: 1, DB: 8},
		{Type: "gain", DB: 12},
		{Type: "drive", Amount: 0.6, Mode: "soft"},
		{Type: "limiter", Threshold: -1},
	},

	// Робот.
	"robot": {
		{Type: "gate", Threshold: -50},
		{Type: "ringmod", Freq: 60, Mix: 0.85},
		{Type: "bitcrush", Bits: 8, Down: 2},
		{Type: "gain", DB: 4},
		{Type: "limiter", Threshold: -1},
	},
}

// PresetOrder — порядок переключения по "preset:next".
var PresetOrder = []string{"clean", "dead_inside", "dead_inside_max", "radio", "radio_broken", "megaphone", "robot"}
