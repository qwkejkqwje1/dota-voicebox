# Свои звуки

Кидайте сюда `.wav` или `.mp3` — они подхватятся **без перезапуска** (раз в 2 секунды).
ID звука = имя файла без расширения: `sounds/mid_gank.mp3` → `sound:mid_gank`.

Привязать к клавише в `config.json`:

```json
"hotkeys": { "F9": "sound:mid_gank" }
```

Привязать к событию игры:

```json
"events": { "death": "sad_trombone@both" }
```
