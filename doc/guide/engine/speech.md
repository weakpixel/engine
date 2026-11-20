# Speech and dialogues

Package `speech` lets entities speak with the voices of the [`github.com/weakpixel/voice`](https://github.com/weakpixel/voice) library: the text of a `gfx.Label` types out in step with musical gibberish on the mixer's dialogue bus (`sound.BusVoice`). A `*.dialogue.json` file written in the library's Voice Editor plays in a dialogue box the same way it plays in the library's own example. The file formats (`*.voice.json`, `*.lang.json`, `*.dialogue.json`) are specified in the library's [guide](https://github.com/weakpixel/voice/blob/main/doc/guide.md).

Speech is opt-in: `game` never imports `speech`, and a game that does not call `speech.Install` pays nothing. It is the only engine package that imports the voice library, and it makes no Ebiten calls.

- [Setup](#setup)
- [Speaking entities](#speaking-entities)
- [Dialogue boxes](#dialogue-boxes)
- [Events](#events)
- [Pausing](#pausing)
- [Assets](#assets)
- [No audio, browsers and costs](#no-audio-browsers-and-costs)
- [Config](#config)
- [Panics](#panics)
- [Not covered](#not-covered)

## Setup

Install speech once, after `game.New` and before the first `Update`:

```go
e := game.New(cfg)
sp := speech.Install(e, speech.Config{})
```

`Install` registers the asset loaders (and the library loaders of the kinds when the engine has a library), starts the built-in synthesizer at the engine's sample rate as a [stream](audio.md#streams) on `BusVoice`, adds the speech system to `StagePost` of the root scene and a check to the audio stage. The returned `*speech.Speech` is also the World resource `ecs.GetResource[speech.Speech]`. Use it from the game goroutine only (systems and `Start` hooks). `Speech.Close` stops everything; it is safe to call twice.

## Speaking entities

An entity with a `speech:Speaker` component speaks with `Say`. If it has a `gfx.Label`, the system sets `Label.Text` to the part of the line said so far, every tick until the line ends (a substring of the line: no allocation).

```go
chirpy := speech.LoadVoice(e.Assets().Global(), "chirpy") // a starter voice, or "talk/bard.voice.json"
npc := ecs.NewEntity3(e.World(),
	gfx.Transform{Pos: gfx.Vec2{X: 40, Y: 200}},
	gfx.Label{Font: font},
	speech.Speaker{Voice: chirpy, Speed: 1})
sp.Say(npc, "Hello there, traveler!", speech.SayOptions{})
```

| `speech:Speaker` field | Meaning |
| --- | --- |
| `Voice` (`gfx.VoiceHandle`, asset kind `voice`) | A `*.voice.json` key or a starter name (`"chirpy"`). Zero, missing or failed: silent, at the default speed. A loading voice delays the line until it is ready. |
| `Speed` | Scales how fast the voice speaks; 0 means 1. |
| `Pan` | Added to the pan of every note, -1 (left) to 1 (right). |

The object `speech:Talker` (`Transform`, `Label`, `Speaker`) places a speaking text in a level.

| Method | Does |
| --- | --- |
| `Say(e, text, SayOptions{Speed, Instant, Voice})` | Speaks `text` (plain: inline voice markup works in dialogues only), replacing the running line. `opts.Voice` speaks with another voice, also on an entity without a `Speaker`. Allocates (it composes the line). |
| `Skip(e)` | Ends the line at once with all text shown. |
| `Stop(e)` | Ends the line where it is. |
| `Pause(e)`, `Resume(e)` | Holds and continues the line ([Pausing](#pausing)). |
| `Speaking(e)` | Whether `e` has a line that has not ended (playing, paused or waiting for its voice). |
| `Revealed(e)` | How many bytes of the line are shown; `text[:n]` is always whole characters. |

A line ends (with a `Stopped` event) when the entity is deleted or loses its `Speaker`.

For showing a line (a piano roll, a karaoke bar), `LineNotes(e, dst)` appends the notes of `e`'s line (`speech.Note{On, Off, Pitch, Velocity, Kind}`, `Kind` one of `NoteVowel`, `NoteSonorant`, `NoteFricative`, `NotePlosive`, `NoteMotif`, `NotePercussion`) and `LinePosition(e)` returns how far it played and its length, both in line time (before `Speed`). The position stays put while the line is paused and stays where `Skip` or `Stop` ended it.

## Dialogue boxes

An entity with a `speech:DialogueBox` component plays a dialogue file: the current line types out into the entity's `gfx.Label` in its speaker's voice, and the speaker's display name goes into the `Label` of the `Name` entity. Both Labels are set to `""` when the dialogue finishes or stops.

| `speech:DialogueBox` field | Meaning |
| --- | --- |
| `Dialogue` (`gfx.DialogueHandle`, asset kind `dialogue`) | The `*.dialogue.json` file. Zero, missing or failed: `Play` plays nothing and sends `DialogueFinished`. |
| `Name` (`ecs.Entity`; in a level the id of an object of the same document) | Optional entity with a `gfx.Label` for the speaker's name. |
| `AutoStart` | Plays from the first line once the dialogue is ready, without a `Play` call (a box placed in a level speaks when the level runs). It starts once per box. |
| `AutoAdvance` | Goes on after every line by itself after the line's hold time, whatever the file says. |

The object `speech:Box` (`Transform`, `Label`, `DialogueBox`) places a box in a level; put a `gfx:Panel` behind it and point `Name` at a `gfx:Text`.

| Method | Does |
| --- | --- |
| `Play(box, start)` | Plays from line `start` (0 is the first), replacing what the box played (its notes are released, no event). Starts at once or, while the dialogue loads, in the system's next tick. A `start` outside the lines is reported to `Config.OnError` and sends `DialogueFinished`. Allocates. |
| `Next(box)` | The game's "advance" input: while a line types out, shows the rest at once; while a finished line holds, goes on to the next line or finishes. Does nothing during a line's delay, while loading and after the end. |
| `StopDialogue(box)` | Ends the dialogue where it is; no event. `Play` starts it again. |
| `PauseDialogue(box)`, `ResumeDialogue(box)` | Holds and continues the dialogue ([Pausing](#pausing)). |
| `Playing(box)` | Whether the dialogue has not finished: speaking, holding, in a delay, paused or loading. |
| `Line(box)` | The current `dialogue.Line` and `dialogue.Speaker`, and whether a line is playing. `Line.Text` is the whole line. |

Input stays the game's: the engine adds no actions. Map your own "advance" action to `Next`, and to `Play(box, 0)` once `Playing` is false if the dialogue should start again:

```go
if st.JustPressed(acts.Advance) {
	if sp.Playing(box) {
		sp.Next(box)
	} else {
		sp.Play(box, 0)
	}
}
```

Each box takes one synthesizer owner per speaker of its dialogue, so boxes and speaking entities never cut each other's notes. A line's inline voices (`{wren}...{/}`) and impressions (`{laugh~}...{/}`) speak with the other speaker's owner. The box ends without an event when the entity is deleted or loses its `DialogueBox`.

## Events

Read `speech.Event{Entity, Kind, Line}` with a `system.EventReader` on `system.EventsOf[speech.Event]`. The speech system sends them in `StagePost`, so a reader in `StageSimulate` sees them the tick after.

| Kind | From | When | `Line` |
| --- | --- | --- | --- |
| `Started` | `Say` | the line began (once its voice was loaded) | the entity's line number, from 1 per `Say` |
| `Completed` | `Say` | the line played to its end | the same |
| `Skipped` | `Say` | `Skip` ended it | the same |
| `Stopped` | `Say` | `Stop`, a new `Say`, `Close`, the deletion of the entity or the loss of its `Speaker` ended it | the same |
| `LineStarted` | box | a line passed its delay and began to speak | the line's index in the file, from 0 |
| `LineDone` | box | a line completed or was skipped by `Next` | the same |
| `DialogueFinished` | box | the last line gave way, or `Play` found nothing to play | the index of the last line played |

`StopDialogue` and the deletion of a box send no event.

## Previewing voices and dialogues

`go tool wpix-editor <file>` opens a `*.voice.json` or a `*.dialogue.json` with sound ([editor.md](editor.md#wpix-editor)): a voice says a line typed in the panel and draws its notes, a dialogue plays line by line with a Next button. In VSCode the sound starts after the first click inside the view. Edit voices and dialogues in the library's Voice Editor.

A tool that runs the engine paused (`game.TickPause` skips `StagePost`) calls `Step(dt)` once per tick instead of the system; a game never needs it.

## Pausing

Three things hold speech. Each releases the sounding notes, so no note hangs, and continues with the next notes:

- A pause scene pushed above the root (or a Tool's pause) freezes every line and every box: the speech system runs in the root's `StagePost`, and a check in the audio stage notices when it did not run. They continue after the pop.
- `Pause(e)` and `Resume(e)` hold one speaking entity, for pause screens that keep the scene below running (`SceneOptions.UpdateBelow`).
- `PauseDialogue(box)` and `ResumeDialogue(box)` hold one box: the line, its delay or its hold. A dialogue that is still loading starts paused. `Next` still works while paused; the next line then waits for `ResumeDialogue`. `Play` resumes.

## Assets

| Kind (library, GOD) | Files | Type | Handle | Load |
| --- | --- | --- | --- | --- |
| `voice` | `*.voice.json`, or a starter name (`"chirpy"`) | `speech.Voice` | `gfx.VoiceHandle` | `speech.LoadVoice`, `LoadVoiceSync` |
| `language` | `*.lang.json` | `speech.Language` | `speech.LanguageHandle` | `speech.LoadLanguage` |
| `dialogue` | `*.dialogue.json` | `speech.Dialogue` | `gfx.DialogueHandle` | `speech.LoadDialogue`, `LoadDialogueSync` |

A voice's `language` field is a `*.lang.json` path relative to the voice file, or a name `<name>` that loads `<name>.lang.json` next to it; `""` is the library's default language. A voice's sample instruments load the WAV files they name, relative to the voice. A dialogue's speaker voices that are files load relative to the dialogue. A reloaded voice takes effect at the next line; a reloaded dialogue is used by the next `Play`. Placeholders: a missing or failed voice is a silent one that still reveals the text at the default speed; a failed dialogue plays nothing. See [asset-kinds.md](asset-kinds.md#voices-and-dialogues).

In the level editor of a game that imports `speech`, a `*.voice.json` or `*.dialogue.json` dropped on the viewport creates a `speech:Talker` or `speech:Box` with the file in its field, and a dropped file fills a voice or dialogue field in the inspector. The level editor runs speech silently. `wpix mcp` checks voice and dialogue fields like other asset fields; a voice field without a folder and without `.json` is taken as a starter name, and an unknown one is reported when the level loads.

## No audio, browsers and costs

Text, reveal and events never depend on audio: under `game.Config.NoAudio` (servers, tests, the level editor) everything behaves the same, silently. While nobody reads the synthesizer's stream (no audio, a browser before the first click), the system drops the queued notes every tick, so they do not play as a burst once audio starts. In a browser, audio starts after the first input.

Costs (the library's [design notes](https://github.com/weakpixel/voice/blob/main/doc/design.md) have the synthesizer's): a tick of running lines and boxes allocates nothing; `Say` and `Play` allocate (composing a line takes about 9 µs natively, 43 µs in js/wasm). The synthesizer renders on the audio pull, which in a browser shares the main thread with frames. The `dialogue` demo in headless Chromium (software GL) keeps the main thread 14 to 16 % busy while two voices speak, against 12.6 to 12.8 % with the dialogue paused: the voices cost about 2 to 3 % of the main thread. The default polyphony stays at 24 voices for that reason.

## Config

| `speech.Config` field | Default | Meaning |
| --- | --- | --- |
| `Bus` | `BusVoice` (the zero `BusSFX` means `BusVoice`) | The mixer bus the voices play on. |
| `Volume` | 1 | The stream's own volume factor. |
| `Voices` | `speech.DefaultVoices` (24) | Polyphony of the synthesizer. |
| `Buffer` | `sound.DefaultStreamBuffer` | The player buffer. |
| `Reverb` | the library's default | The synthesizer's reverb. |
| `Sink` | the built-in synthesizer | Receives the notes instead (a recording sink in tests); no stream is started. |
| `OnError` | logs | Receives what goes wrong while speaking (a voice the synthesizer rejects, a stream that cannot start, a `Play` start outside the lines). Asset errors go to the asset server's `OnError`. |

## Panics

| Message | Cause |
| --- | --- |
| `speech: Say on a dead entity` | `Say` on an entity that is not alive. |
| `speech: Say on an entity without a Speaker and without SayOptions.Voice` | Nothing to speak with. |
| `speech: Play on a dead entity` | `Play` on an entity that is not alive. |
| `speech: Play on an entity without a DialogueBox` | `Play` on an entity without the component. |
| `speech: more than 65536 speakers` | Every synthesizer owner is taken. |
| `speech: <error>` | `Install` with an invalid `Config.Voices` or `Config.Reverb`. |

## Not covered

- Choices and branches in dialogues, portraits, several dialogues in one box, overlapping lines.
- Positional speech (panned by the `Listener`), ducking music under dialogue, rhythm voices synced to the playing music.
- Inline voice markup in `Say` (dialogues only), Lua bindings.

