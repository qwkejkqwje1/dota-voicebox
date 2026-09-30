# 🎙 Dota VoiceBox

Саундпад для Dota 2: звуки **в голосовой чат** по горячим клавишам, **пресеты искажения голоса**
(«дед инсайд», рация, мегафон, робот) и **звуковые таймеры рун** по игровому времени через
официальный Game State Integration.

Никакого вмешательства в игру: не читает память, не внедряется в процесс — только аудио,
горячие клавиши и официальный GSI от Valve.

## Возможности

- 🔊 Звуки в войс по глобальным горячим клавишам (работают поверх игры), ваш голос идёт как обычно.
- 🚨 Встроенная **сирена** «мид ганкают» (`Num1`).
- ⏱ Таймеры по игровым часам Dota: **руна мудрости (экспа)** каждые 7:00 и **руна в миду на 6:00**;
  силовые, баунти-руны и стаки — включаются в конфиге. Пауза и переподключение не сбивают таймеры.
- 🐉 Таймер Рошана по клавише: аегис 5:00, окно респауна 8:00–11:00.
- 🎛 Пресеты голоса, переключаются на лету:

  | Пресет | Звук |
  |---|---|
  | `clean` | чистый голос |
  | `dead_inside` | передавленный, перегруженный микрофон с раздутым басом |
  | `dead_inside_max` | ушная боль: жёсткий клиппинг, 6 бит, заворот волны |
  | `radio` | рация: полоса 350–3200 Гц, щелчок в начале фразы, шипение, «хвост» в конце |
  | `radio_broken` | рация с плохим приёмом: треск, выпадения, цифровая грязь |
  | `megaphone` | громкоговоритель |
  | `robot` | робот (кольцевая модуляция) |

- 📁 Свои звуки: кинули `.mp3`/`.wav` в `sounds/` → сразу доступны, конфиг перечитывается на лету.
- 🗣 Озвучка текста (Windows TTS): `"files": ["tts:Мид пропал!"]`.
- 🎮 Звуки на события игры: смерть, килл, серия, низкое HP, 6 уровень, смок, день/ночь, победа/поражение.
- 🎧 Самопрослушка (`NumMul`) — слышать свой обработанный голос, чтобы подобрать пресет.
- ⌨️ Автонажатие кнопки голосового чата, пока играет звук.

## Установка

1. Установите **[VB-Audio Virtual Cable](https://vb-audio.com/Cable/)** (бесплатно), перезагрузите ПК.
2. Скачайте `voicebox.exe` из [Releases](../../releases) или из артефактов последней сборки в **Actions**.
3. Запустите `voicebox.exe` — рядом создастся `config.json`.
4. В `config.json` укажите `"ptt": { "key": "..." }` — **ту же клавишу, что стоит на голосовом чате в Dota 2**.
5. Dota 2 → Настройки → Звук → **Устройство ввода: `CABLE Output (VB-Audio Virtual Cable)`**.
6. Таймеры: `voicebox.exe -install-gsi` и параметр запуска Dota 2 в Steam: `-gamestateintegration`.

Проверить устройства: `voicebox.exe -list-devices`. Если микрофон не по умолчанию — впишите часть его имени в `devices.mic`.

```
Микрофон ──► [пресет голоса] ──┐
                               ├──► CABLE Input ──► Dota 2 (микрофон = CABLE Output)
Звуки (войс) ──────────────────┘
Звуки (напоминания) ─────────────► ваши наушники
```

## Горячие клавиши по умолчанию

| Клавиша | Действие |
|---|---|
| `Num1` | 🚨 сирена (в войс) |
| `Num2` / `Num3` | сообщить команде про руну мудрости / руну на 6:00 |
| `Num0` | чистый голос |
| `Num4`…`Num9` | dead_inside, dead_inside_max, radio, radio_broken, megaphone, robot |
| `Num+` | следующий пресет |
| `Num-` | Рошан убит (запуск таймеров) |
| `Num*` | самопрослушка вкл/выкл |
| `Num.` | остановить все звуки |
| `Num/` | перечитать конфиг |

Формат: `"Ctrl+Alt+F9"`, `"Shift+Num1"`, `"F13"`.
Действия: `sound:<id>`, `sound:<id>@both|voice|monitor`, `preset:<имя>`, `preset:next`, `preset:prev`, `stop`, `rosh`, `mic_monitor`, `reload`.

## Конфиг

Полный пример — [`config.example.json`](config.example.json). Главное:

### Звуки

```json
"sounds": {
  "siren":    { "files": ["builtin:siren"], "bus": "both", "cooldown": 3, "mode": "interrupt" },
  "gank":     { "files": ["sounds/gank1.mp3", "sounds/gank2.mp3"], "volume": 0.8 },
  "mid_miss": { "files": ["tts:Мид пропал!"], "bus": "voice" }
}
```

- `files` — несколько файлов = случайный выбор. Источники: путь к файлу, `builtin:<имя>`, `tts:<текст>`.
- `bus`: `both` — команде и вам, `voice` — только команде, `monitor` — только вам.
- `mode`: `overlap` (накладывать), `interrupt` (перезапустить), `ignore` (не запускать, пока играет).
- `cooldown` — секунды между повторами (антиспам).
- Встроенные: `siren`, `wisdom_rune`, `mid_rune_6`, `power_rune`, `bounty_rune`, `rosh`, `stack`, `beep`, `danger`.
  Послушать/взять за основу: `voicebox.exe -export-sounds builtin_wav`.

### Таймеры

```json
"timers": [
  { "id": "mid_rune_6",  "sound": "mid_rune_6",  "at": [360], "warn_before": 10 },
  { "id": "wisdom_rune", "sound": "wisdom_rune", "start": 420, "every": 420, "warn_before": 20 },
  { "id": "power_rune",  "sound": "power_rune",  "start": 360, "every": 120, "skip": [360], "warn_before": 10, "disabled": true }
]
```

Время — секунды игровых часов. Интервалы рун меняются от патча к патчу — правьте здесь, без пересборки.

### События

```json
"events": { "death": "sad_trombone@both", "streak_3": "rampage", "low_hp": "low_hp" }
```

Доступно: `game_start`, `pregame`, `kill`, `death`, `respawn`, `streak_3`, `streak_5`, `streak_10`,
`low_hp`, `level_6`, `smoked`, `day`, `night`, `victory`, `defeat`.

### Свои пресеты голоса

```json
"voice_presets": {
  "my_bass": [
    { "type": "gate", "threshold_db": -50 },
    { "type": "lowshelf", "freq": 150, "db": 10 },
    { "type": "gain", "db": 15 },
    { "type": "drive", "amount": 0.8, "mode": "soft" },
    { "type": "limiter", "threshold_db": -1 }
  ]
}
```

Эффекты: `gain`, `lowpass`, `highpass`, `bandpass`, `peaking`, `lowshelf`, `highshelf`,
`drive` (`soft|hard|fold`), `bitcrush`, `compressor`, `gate`, `squelch` (рация), `noise` (`white|crackle`),
`dropout`, `ringmod`, `limiter`. Встроенный пресет можно переопределить, задав его имя.

## Сборка

Нужен Go 1.22+ и компилятор C (cgo для miniaudio): на Windows — [MSYS2/mingw-w64](https://www.msys2.org/) или TDM-GCC.

```bash
go test ./...
go build -o voicebox.exe ./cmd/voicebox
```

CI (GitHub Actions) собирает `voicebox.exe` на каждый push; тег `v*` создаёт релиз.

## Структура

```
cmd/voicebox        точка входа, флаги
internal/audio      движок (malgo/miniaudio): дуплекс мик→кабель, наушники, микшер
internal/dsp        эффекты и пресеты голоса
internal/sounds     WAV/MP3, встроенные синтезированные звуки, библиотека
internal/timers     таймеры по игровому времени
internal/gsi        приём GSI и игровые события
internal/winapi     горячие клавиши, эмуляция PTT, TTS, поиск Dota 2
internal/app        связка всего, горячая перезагрузка
```

## Честная игра

Не спамьте: у звуков есть `cooldown`, а тиммейты могут вас замьютить или зарепортить.
Напоминания таймеров по умолчанию слышите только вы.
