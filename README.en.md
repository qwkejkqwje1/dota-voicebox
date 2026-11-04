# 🎙 VoiceBox

[Русский](README.md) · **English**

**Soundboard and voice effects for Windows.** Sounds and your voice with an effect go to a virtual microphone —
everyone you talk to hears them: **Discord, Dota 2, CS2, TeamSpeak, Telegram, OBS** and any other app.

- 🔊 **Sounds** — tiles and hotkeys. Your own files (drop them into the window), **recording from the microphone**,
  phrases spoken by a Windows voice, ready-made sounds. **Trimming**: the app cuts silence at the edges by itself,
  anything else — with handles on the waveform. **Sets for games**: separate tiles for Dota 2, CS2 and so on — a set
  turns on by itself when its game is on screen.
- ⚡ **Quick menu over the game** — press `Alt+~`, pick a sound with a number key or the mouse without minimizing
  the game. `Enter` — search by name, and any phrase you type is spoken to your team by a Windows voice.
- 🎯 **Sound wheel** — like the Dota 2 chat wheel: hold a key, move the mouse towards a sound, release.
- 🎭 **Reader** — type a phrase and a sly, nasty, demonic or a dozen other character voices read it out.
  Over the game with `Ctrl+Alt+R`, in the quick menu and on the wheel. It can also read Dota 2 chat aloud.
- 📱 **Phone remote** — sound tiles on your phone screen over home Wi-Fi: scan the QR code and you're done.
- 📻 **Voice effects** — a proper walkie-talkie with hiss and click, military radio, Mission Control, phone, robot,
  demon, Auto-Tune and a dozen more.
- 🎚️ **Clean voice** — noise suppression (the RNNoise neural network removes keyboard and fan noise), timbre and
  equalizer, even loudness and a background under your voice: rain, campfire, sea, outer space…
- 🐲 **Dota 2 module** (optional) — a “Gank mid!” button, voice reminders about runes, Roshan,
  your own anthem on victory and a sound on defeat.
- 🌐 **English and Russian interface** — VoiceBox follows the Windows language; switch it in the setup wizard or in
  “Settings” → “🖥️ App” → “Interface language”.

No tampering with games: VoiceBox doesn't read game memory or inject into processes — only audio, global hotkeys
and the official Valve integration (GSI).

## How it works

```
🎙 Microphone ─► voice effect ──┐
🔊 Sounds, phrases, reminders ──┼─► 🔌 CABLE Input ══ CABLE Output ─► 💬 Discord · 🎮 game · 🎥 OBS
                                └─► 🎧 your headphones (you hear the sounds too)
```

You need the free virtual cable [VB-Cable](https://vb-audio.com/Cable/) — VoiceBox offers to install it for you.
Other cables work too: Virtual Audio Cable (“Line 1”), Voicemeeter and so on.

## Quick start

1. Download `dota-voicebox-…-windows.zip` from [Releases](../../releases), unpack it to any folder
   (for example `Documents\VoiceBox`) and run `voicebox.exe`. **Nothing to install** — the app and its settings
   live in that folder.
2. The **setup wizard** opens: interface language, microphone and headphones, virtual cable, “Do you play Dota 2?”,
   a Start menu shortcut and autostart.
3. In the apps where people should hear you, choose the **CABLE Output** microphone:
   - Discord → User Settings → Voice & Video → Input Device;
   - Dota 2 → Settings → Audio → Input device;
   - OBS → an “Audio Input Capture” source.
4. Done. Closing the window doesn't quit the app — VoiceBox lives in the tray (right-click the icon for effects,
   microphone, stop, the Dota 2 module), and hotkeys work in games too.

Requires Windows 10/11 (WebView2 is already part of the system; if it's missing, the interface opens in a browser).

**Shortcuts** — in “Settings”: a Start menu shortcut (Windows Search finds VoiceBox), a desktop shortcut
and “Start with Windows”. Moved the app folder? Shortcuts and autostart are updated on the next launch.
To remove VoiceBox, turn off the shortcuts and autostart and delete the folder.

## Sounds

- **Add**: drop files into the window (MP3, WAV, OGG, FLAC, M4A…), type a phrase for a Windows voice to speak,
  or take a ready-made sound: party horn, sad trombone, ba-dum-tss, drum roll, crickets, “correct/wrong”,
  pew-pew, boing, sirens and signals.
- **Click a tile** and everyone hears the sound; 🎧 — listen yourself; **right-click** — settings:
  name, icon, color, key, volume, “who hears” (all / on air only / only me),
  “no more than once per N seconds”, what a repeated press does, group, whether to show it in the quick menu.
- **🎙️ Record** (above the pad, or “＋ Add” → “Recording”): press the red button and say a phrase, a battle cry,
  a laugh — up to a minute. Noise suppression works here too, and the app trims silence and mouse clicks at the edges.
  Before saving: a waveform with handles, “▶ Listen”, a name and, optionally, “With the voice effect” — the sound
  is recorded the way your team hears you.
- **✂️ Trim** any sound from a file: “Sound settings” → “✂️ Trim” — drag the start and end handles
  (or use the `←` `→` arrows), type the time or press “Remove silence”. The file isn't changed — “↺ No trim”
  brings the whole sound back.
- Tiles can be dragged around — the order is saved and used in the quick menu.
- `Pause` — stop all sounds.

### Sound sets

Different games — different sounds: “🗂️ Sets for games” above the pad. A set has its own tiles in its own order,
and the active set is what the pad, the quick menu, the wheel (unless you pick sectors by hand) and the phone remote show.

- **Auto by game**: tick the games for a set (Dota 2, CS2, Valorant, League of Legends, Deadlock, Fortnite, Apex,
  PUBG, Overwatch 2, Rust, GTA V, Roblox — or any app: recently opened ones appear in the list by themselves).
  When the game is on screen, its set turns on.
- **By hand**: with a click above the pad, from the phone or with the “Next sound set” key right in the game.
- One sound can be in several sets (“Sound settings” → “In which sets”); new sounds go into the active set.
  “🎛️ All sounds” is the whole regular pad.

## Quick menu over the game

`Alt+~` (change it on the “In-game menu” page) opens a compact menu over the game. Three layouts:

- **List** — rows with names and keys;
- **Strip** — tiles in one row at the top or bottom of the screen: barely covers the game;
- **Grid** — large 5×2 tiles, like a streamer's control deck.

| Key | Action |
|---|---|
| `1`…`0` (and the numpad) | play a sound / turn on an effect |
| mouse, wheel | pick / scroll pages |
| arrows and `Enter` | pick: `↑` `↓` in the list, `←` `→` in the strip, all four in the grid |
| `←` `→` in the list, `↑` `↓` in the strip, `PgUp` `PgDn` everywhere | pages (10 sounds each) |
| `Tab` | the “Voice” tab — switch the voice effect |
| `Enter` (nothing selected) | search and phrase to the team — see below |
| `Backspace` | stop sounds |
| `Esc` or the same key | close |

**Search and phrase to the team.** Press `Enter` in the open menu (or your own “Search and team phrase over the game”
key — the menu opens with search right away) and start typing: it finds every sound on the pad and every voice
effect. `↑` `↓` and `Enter` play or turn it on, **`Ctrl+Enter` — a Windows voice speaks the typed phrase** and your
team hears it (recent phrases are remembered). It understands both English and Russian keyboard layouts, even if you
forgot to switch, and `ё` = `е`. While search is open, key presses don't reach the game; `Esc` goes back to the menu.

**Where it appears**: 9 ready-made positions (center, top, bottom, corners and sides), “at the cursor” or your own
spot. Set your own spot by dragging — on the screen diagram in the settings or right in the game: pull the menu by
the ⠿ handle or the background, and VoiceBox remembers the place. With several monitors the menu opens where the game
is (or where the cursor is, or on the primary one). You can also set the size, background opacity, auto-close and
what the menu contains.

Any other key closes the menu and goes to the game.

> The menu is visible over games in **borderless windowed** mode. In exclusive fullscreen Windows doesn't allow
> drawing over the game — sound hotkeys still work there.

## Sound wheel

Like the Dota 2 chat wheel, but with sounds: **hold the key — 8 sectors appear around the cursor, move the mouse
towards the one you need and release**, and the sound plays. No need to move the mouse far; if you don't move it,
nothing happens.

- **Turn on**: the “Wheel” page → the switch. It immediately asks for a key: take one that does nothing in the game
  (for example, your former chat wheel key) or a side mouse button.
- **Tap** the key and the wheel stays on screen: click a sector or press a number `1`…`8`.
  `Esc` or the right mouse button closes it. It closes by itself after 8 seconds of inactivity.
- **What's in the sectors**: by default, the first 8 sounds from the pad (or the active set). “Custom” — any sounds,
  voice effects, “Stop” and “Microphone” in any order; sectors are rearranged by dragging on the wheel diagram.
- **Look**: at the cursor or in the center of the screen, size, opacity. “▶ Show now” opens the wheel to take a look.

> Like the quick menu, the wheel is visible over games in **borderless windowed** mode.

## Reader

Type a phrase and a **character voice** reads it: a Windows voice with its own speed, pitch and processing.
Your team (through the virtual microphone) and you hear it, only the team or only you.

| Voice | How it sounds |
|---|---|
| 😏 Sly | drawls, nasal, with a smirk |
| 🤢 Nasty | nasal, gurgling and obnoxious |
| 👹 Demon · 🗿 Deep | a low growl from the underworld · low and solid |
| 🤖 Robot · 👽 Alien | a metallic machine voice · a floating inhuman voice |
| 🐿️ Chipmunk · 👴 Grandpa · 🥴 Drunk | squeaky and fast · old and trembling · wobbles and drags words |
| 📻 Walkie-talkie · 🏟️ Announcer | with hiss, like over a radio · a stadium-wide announcement |
| 🙂 Plain · 🎛️ My effect | the Windows voice without processing · your current voice effect and timbre |

- **Over the game**: `Ctrl+Alt+R` opens a typing line with the selected voice on the right. Type, `↑` `↓`
  (or `←` `→`, the mouse wheel) pick the voice, `Enter` reads it, `Ctrl+Enter` reads it only to you, `Esc` closes.
  While nothing is typed, the rows show recent phrases: `Enter` reads the selected one again.
- **In the quick menu**: `Enter` → type the text → the “Read as…” row. **On the sound wheel** — the “Reader”,
  “Repeat phrase”, “Reader voice” and “Chat reading” sectors.
- **On the “Reader” page**: a text box (`Enter` — say, `Shift+Enter` — new line), click a voice to pick it and
  hear a sample (only you), phrase history, 🔁 repeat and 🤐 stop reading, your own Windows voice, speed
  and volume. Keys: “Repeat phrase”, “Next voice”, “Stop reading”.
- Russian text is read by a Russian Windows voice and English text by an English one, if installed.

**💬 Reading Dota 2 chat aloud** (a card on the “Reader” page, needs the Dota 2 module): a character voice reads
messages from the match chat — no need to look away to read. All chat and/or team chat, optionally your own
messages and chat wheel phrases; long messages are trimmed. Messages are queued and don't interrupt each other,
“Stop reading” clears the queue. Usually only you hear it. Key: “Dota 2 chat reading: on/off”. Want allies in one
voice and enemies in another? See the `chat_reader` example script (the “📜 Example script” button on the page).

## Phone remote

Your phone turns into a soundboard: sound tiles, a “Voice” tab with effects and a microphone button — right in the
phone's browser, no apps or internet needed. If you use sound sets, a set switcher sits above the tiles.

1. The “Phone” page → turn on **“Phone remote”**.
2. Connect the phone to the **same Wi-Fi** as the computer (the computer may be connected to the router by cable).
3. Point the phone camera at the QR code and open the link. To keep the remote at hand, add it to the home screen:
   iPhone — “Share” → “Add to Home Screen”, Android — browser menu → “Add to Home screen”.

The first time, Windows asks whether to let VoiceBox on the network — click **“Allow access”** (private networks are enough).

**The phone can't open the remote?** The “Phone” page has a step-by-step check:

- the phone and the computer are on the same network (not mobile data and not the router's guest Wi-Fi);
- the network in Windows is **“Private”**: on a “Public” network Windows doesn't let other devices reach the
  computer (the “Open network settings” button);
- firewall: the **“🛡️ Allow in firewall”** button adds one narrow rule — only VoiceBox, only the remote's port,
  only private networks and devices from your network (Windows asks for confirmation). If you once clicked
  “Cancel” in the firewall window, tick “Private” next to VoiceBox in the list of allowed apps;
- a VPN on the computer or the phone may block the home network — turn it off for a while;
- the port is taken by another app — change it (by default it's the interface port + 1, usually `3002`).

**Security**: the remote only lets in devices from the local network, and only with the link containing the secret
key; after 50 wrong attempts an address is blocked for 10 minutes. The list of phones that connected is on the
“Phone” page. “🔑 New link” changes the key — old links and QR codes stop working. When the remote is off,
it doesn't listen on the network.

## Voice effects

| Group | Effects |
|---|---|
| Basic | Clean, Studio |
| Radio & comms | **Walkie-talkie**, Military radio, Bad connection, Mission Control (Quindar tones), Phone, Megaphone |
| Characters | Robot, Demon, Low voice, Chipmunk, Alien |
| Music | **Auto-Tune** — pulls your voice to notes (notes, key, smoothness and strength are adjustable), Robo-tune — hard, in “steps” |
| Space | Stadium announcer, Cave, Underwater |
| Memes | Dead Inside, Ear pain |

Effects have knobs. The walkie-talkie, for example: static noise, overdrive, band narrowness, interference,
“digital grit”, a click at the start and a “pssh” at the end of a phrase, a tone at the end (beep, double, chirp,
Quindar) and at the start of a transmission. Turn on “Hear yourself” on the “Voice” page to tune the sound —
changes apply instantly, without clicks. Also there: microphone gain, noise gate, waveform and “before/after” spectrum.

**Voice processing** (the “✨ Enhance voice” card on the “Voice” page, works together with any effect):

- **🔇 Noise suppression** — the [RNNoise](https://github.com/xiph/rnnoise) neural network removes keyboard,
  mouse, fan, hum and street noise. Strength: “Gentle”, “Normal”, “Maximum”. Runs on the CPU (about 2% of one core),
  latency ~20 ms; when it's off, the voice goes through with no delay.
- **Timbre**: Natural, Warm, Velvet, Radio host, Crisp, Deep, Podcast — or “Custom”: a 5-band equalizer and
  **“Even loudness”** (quiet words get louder, shouting doesn't hurt ears).
- **Background under voice**: rain, thunderstorm, wind, campfire, sea, outer space, vinyl or any sound of your own
  on a loop. Volume, “Only while I talk” — the background plays under your speech and fades in pauses (turn it off
  and it plays all the time while the microphone is on), “🎧 Listen”. Key: “Background under voice: on/off”.

**The microphone** is chosen right on the “Voice” page (and in the tray icon menu); the “Next microphone” key cycles
through them. If the selected microphone is unplugged, VoiceBox warns you and temporarily uses the Windows default
microphone, and when it's plugged back in, VoiceBox switches back to it by itself.

**When you're heard**: always, while you hold a key, or “press — on, press again — off”.
If push-to-talk is on in the game or Discord, set the **voice chat key** in “Settings” —
VoiceBox holds it down while a sound plays (for Dota 2 it can be taken from the game with one button).

## Dota 2 module

Turned on with one switch on the “Dota 2” page (or in the wizard). While it's off, its buttons and keys don't get in the way.

- **Alert buttons** for the team: 🚨 “Gank mid!” (`Num1`), ❓ “Mid is missing” (`Ctrl+Num1`), Smoke!,
  Roshan, Everyone defend — a signal + voice. You can add your own.
- **Voice reminders by game time** — with time limits, just like in the game: a wisdom rune every
  7 minutes (20 s ahead), **power runes at 6:00** (10 s ahead); optionally water runes (2:00, 4:00), bounty runes,
  **lotuses until 9:00**, stacking camps until 20:00. Your own reminders take two clicks: “every…”, “first time at…”,
  “until…” or an exact time. `{time}` in a phrase is spoken as “in 40 seconds”. Who hears: only you or you and the team.
- **Supports to runes**: 40 seconds before 6:00 — “Supports, power runes in 40 seconds. Pull and secure the runes” —
  the whole team hears it.
- **Roshan**: “Roshan killed” → reminds you when the Aegis expires and when Roshan may respawn.
- **🏆 Event sounds** — no scripts needed: your own anthem on **victory** and a sound on **defeat**. Pick a sound from
  the pad or load a file right in the row (📁); who hears — all or only you. It plays once, as soon as the game shows
  the winner (silent when you're spectating). “＋ More events”: kill, death, respawn, kill streaks, low HP,
  level 6, smoke, day and night, the 0:00 horn, Roshan.
- **💬 Chat aloud**: the Reader's voice reads messages from the match chat — see [Reader](#reader).
- **Auto setup**: connection to the game (GSI), the launch option, the voice chat key.
  Without a connection to the game you can press “Horn 0:00” or set the time by hand.

## Default hotkeys

| Key | Action |
|---|---|
| `Alt+~` | quick menu over the game |
| `Pause` | stop all sounds |
| `Ctrl+Alt+V` | open the VoiceBox window |
| `Ctrl+Alt+R` | Reader over the game |
| `Num1` / `Ctrl+Num1` | “Gank mid!” / “Mid is missing” (Dota 2 module) |

The sound wheel key is assigned when you turn the wheel on. Hotkeys work everywhere, including in games.
Any sound and effect can be put on a key, a key combination or a side mouse button. All keys are in “Settings”:
including “Search and team phrase over the game”, “Noise suppression: on/off”, “Background under voice: on/off”,
“Next sound set”, “Next microphone”, the Reader keys and “Dota 2 chat reading: on/off”.

## Advanced

**Scripts** (the “Scripts” page): JavaScript with your own keys, combos, timings and game events.

```js
combo("Num2 Num2", () => say("Going for the rune", { to: "team" }), { within: 400 });
every("7:00", (t) => notify("Wisdom at " + fmt(t)), { before: 20 });
on("death", () => play("sad_trombone", { to: "me" }));
on("chat", (m) => { if (!m.own) read(m.text, { voice: m.ally ? "announcer" : "vile", to: "me" }); });
```

API: `hotkey`, `combo`, `hold`, `onKey`, `at`, `every`, `on`, `after`, `repeat`, `cancel`, `play`, `say`, `read`, `stop`,
`preset`, `action`, `notify`, `log`, `time`, `fmt`, the `game` object (`clock`, `hero`, `paused`, `raw`…). The reference
is in the editor. With the English interface, the example scripts are in English too.

**Config**: everything is set up in the interface; `config.json` next to the app can also be edited by hand — changes
are picked up on the fly, and a backup is made before every save (restore it in “Settings”).
Old configs (v1–v4) are converted to the new format automatically. Example: `config.example.json`.

**Extra sources** (“Settings” → at the bottom): music or a second microphone on air.

**Check** (the “Check” page): status, auto setup, tests for headphones, microphone, cable, effect,
“how I sound”, tips for Discord and other apps, a diagnostic bundle for bug reports.

## Building

Go 1.22+ and a C compiler (cgo for miniaudio): on Windows — [MSYS2/mingw-w64](https://www.msys2.org/) or TDM-GCC.

```bash
go test ./...
go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=v1.5.0" -o voicebox.exe ./cmd/voicebox
```

Cross-compiling from Linux: `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-clang go build …` (llvm-mingw).
Working on the interface without Windows: `VOICEBOX_DEMO=1 VOICEBOX_DEV_TOKEN=dev VOICEBOX_UI_DIR=internal/ui/static go run ./cmd/voicebox`
and open `http://127.0.0.1:3001/?t=dev` (demo devices, interface files are read from disk).

The interface strings in the source code are Russian; the English translation is the dictionary
`internal/i18n/en.json` (key — the Russian string, value — the English one).

CI (GitHub Actions) builds `voicebox.exe` on every push; a `v*` tag creates a release (the version also goes into
the exe properties). A release contains `voicebox.exe.sig` — an ed25519 signature; auto-update installs only signed
builds (the CI key is the `UPDATE_SIGNING_KEY` secret, a new key pair: `go run ./tools/sign -genkey`).
Code signing — see below.

## Code signing

Releases can be signed with an Authenticode certificate **for free** through [SignPath Foundation](https://signpath.org)
(a program for open source projects). For a signed `voicebox.exe`, Windows shows the publisher, and SmartScreen and
antivirus programs complain less about an “unknown app” (reputation builds up with downloads, so a warning may still
appear at first).

How to turn it on (once, done by the repository owner):

1. Add a `LICENSE` file with an OSI-approved license (for example MIT) to the repository — a SignPath Foundation
   requirement. The project has no license yet; the choice is up to the author.
2. Turn on two-factor authentication on GitHub and apply at [signpath.org/apply](https://signpath.org/apply):
   repository `qwkejkqwje1/dota-voicebox`, build system — GitHub Actions (`.github/workflows/build.yml`).
3. After approval, in SignPath: project `dota-voicebox` with the GitHub.com trusted build system, signing policy
   `release-signing`, artifact configuration `initial` — the contents of [`docs/signpath/artifact-configuration.xml`](docs/signpath/artifact-configuration.xml).
   Create a CI user and its API token.
4. GitHub → Settings → Secrets and variables → Actions: the `SIGNPATH_API_TOKEN` secret and the
   `SIGNPATH_ORGANIZATION_ID` variable (the organization id in SignPath). If the names in SignPath differ, use the
   `SIGNPATH_PROJECT_SLUG`, `SIGNPATH_POLICY_SLUG` and `SIGNPATH_ARTIFACT_CONFIG` variables.
5. From then on it's automatic: a `v*` tag → CI sends `voicebox.exe` for signing, waits for approval in SignPath
   (up to an hour) and publishes the already signed file in the release; the `voicebox.exe.sig` update signature is
   computed from the signed exe. Until the secret and the variable exist, releases are built as before — unsigned.

### Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org).

- Only `voicebox.exe` built by GitHub Actions from the sources in this repository is signed. VoiceBox doesn't sign
  third-party programs (VB-Cable) and doesn't put them into the archive.
- Roles: committers and reviewers — [qwkejkqwje1](https://github.com/qwkejkqwje1); approvers —
  [qwkejkqwje1](https://github.com/qwkejkqwje1). Every release signing request is approved manually.
- **Privacy policy**: VoiceBox doesn't transfer any information to other networked systems unless specifically
  requested by the user or the person installing or operating it:
  - checking for and downloading updates from GitHub (`api.github.com`, `github.com`) — turned off in
    “Settings” → “⬆️ Updates”;
  - downloading the VB-Cable driver from `vb-audio.com` — only with the cable install button;
  - the phone remote — only on the local network and only while it's on.

  The interface runs on `127.0.0.1` and isn't visible from outside. No telemetry, no analytics.

## Project structure

```
cmd/voicebox        entry point, window, tray, quick menu, sound wheel
internal/audio      engine (miniaudio): microphone → effect → cable, headphones, sources, smooth effect switching
internal/dsp        voice effects and presets (radio, pitch, reverb, Auto-Tune, …), timbres, backgrounds
internal/sounds     WAV/MP3, built-in synthesized sounds, library, phrases, trimming and silence detection
internal/rnnoise    RNNoise noise suppression (Xiph.Org C sources, BSD-3)
internal/overlay    drawing of the quick menu (with search), the sound wheel and notifications over the game (pure Go)
internal/timers     reminders by game time
internal/gameclock  smoothed game time (GSI + extrapolation + manual mode)
internal/gsi        GSI receiver and game events
internal/script     JS scripts (goja)
internal/combo      combos, double presses, holds
internal/winapi     keys, tray, menu window, voice chat key emulation, TTS, Dota 2 detection
internal/app        ties it all together: sounds, microphone recording, Reader, menu, Dota 2 module, auto setup, tests
internal/config     config v5, migration from old versions
internal/i18n       English interface: the en.json dictionary and translation of server responses
internal/ui         local API + SSE, static/ — the interface (HTML/CSS/JS modules), phone remote (static/remote/)
internal/qr         QR codes for the remote (pure Go)
internal/update     auto-update: check, signature, install, rollback
```

The interface is a local server on `127.0.0.1:3001` with a one-time token (websites in a browser can't control the
app). The phone remote is a separate small server (usually port `3002`): it runs only when turned on and lets in
only devices from the local network with the secret key in the address.

## Fair play

Don't spam: sounds have “no more than once per N seconds”, and teammates can mute or report you.
By default, only you hear the reminders.

## Licenses

Noise suppression — [RNNoise](https://github.com/xiph/rnnoise) © Xiph.Org Foundation, Mozilla, Jean-Marc Valin
et al., BSD-3-Clause license (the text is in `internal/rnnoise/COPYING`, in the app archive — `licenses/rnnoise.txt`).
