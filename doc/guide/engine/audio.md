# Audio

Package `sound` plays sound effects and music. Effects and tracks are assets; gameplay asks for sounds by pushing requests into a queue, and one system plays them at the end of the tick, the same way rendering works.

```go
hit := sound.LoadSound(e.Assets().Global(), "sfx/hit.wav")
theme := sound.LoadMusic(e.Assets().Global(), "music/level1.ogg")

e.Sound().PlayMusic(theme, 2*time.Second)          // crossfade in
e.Sound().Play(sound.PlaySound{Sound: hit})       // from any system
e.Mixer().SetVolume(sound.BusMusic, 0.5)          // the options slider
```

`game.Engine` wires all of it. Wiring it by hand is under [Without game.Engine](#without-gameengine).

## Sounds and music

| Kind | Asset | Load with | Handle | Kept in memory as | Use it for |
| --- | --- | --- | --- | --- | --- |
| `sound` | `sound.Sound` | `sound.LoadSound` | `gfx.SoundHandle` | decoded PCM (16-bit stereo) | Short effects: hits, steps, clicks. Starting one costs nothing but a player. |
| `music` | `sound.Music` | `sound.LoadMusic` | `gfx.MusicHandle` | the compressed file | Long tracks. Decoded while playing, so a 3-minute song costs its file size, not 30 MB. |

Files: `.wav` (PCM), `.ogg` (Vorbis), `.mp3`, chosen by extension. Both kinds decode on the asset workers: a sound fully, music only its header, so a broken track fails at load time, not when it starts. Everything is resampled to the engine's sample rate (`Config.SampleRate`, default 48000).

The placeholder of both kinds is **silence**. A sound that is still loading, failed, or a zero handle plays nothing and never panics, like a missing sprite draws the checker.

## Playing effects

Push a `sound.PlaySound` into the queue (`e.Sound()`, or the `*sound.Queue` resource):

| Field | Meaning |
| --- | --- |
| `Sound` | The effect. If it is not `Ready` this tick, the request is dropped, not deferred: a hit sound a second late is worse than none. |
| `Volume` | Factor. **Zero means 1**, like `Animator.Speed`; negative is silent. |
| `Pan` | -1 left to 1 right. Ignored when `Positional` is set. |
| `Bus` | Mixer bus; zero is `BusSFX`. |
| `Positional`, `At` | Take pan and distance attenuation from the world position `At` and the [listener](#positional-sound). |

The `AudioSystem` drains the queue once per tick and applies two limits from the mixer:

- `SameSoundPerTick` (default 2): ten enemies hit in the same tick play the hit sound twice, not ten times as loud.
- `MaxVoices` (default 32): effect voices playing at once. Requests over it are dropped and counted in `Stats().Dropped`.

## Music

`Queue.PlayMusic(track, fade)` switches the music, crossfading over `fade` (0 cuts). Music always loops and plays on `BusMusic`.

- Asking for the track that is already playing does nothing, so every level can request its track without restarting it.
- If the new track is still loading, the current one keeps playing until it is ready.
- A zero, failed or empty track stops the music, like `StopMusic(fade)`.
- The last music request of a tick wins.

## Buses and the mixer

A bus is a volume that a group of voices shares: the sliders of an options screen. `sound.Mixer` (`e.Mixer()`, or the `*sound.Mixer` resource) has a master volume and 8 buses; `BusSFX`, `BusMusic`, `BusUI` and `BusVoice` are predefined, the rest are yours. A voice's volume is request volume × bus × master (× fade for music, × distance for positional sounds), and **changes reach playing voices on the next tick**.

The zero `Mixer` is silent; `sound.NewMixer` sets every volume to 1. The `AudioSystem` creates one if the world has none.

## Positional sound

`sound.Listener` is where the player hears from. `game.Engine` moves it to the camera every tick and sets `HalfWidth` to half the visible world width, so a sound at the screen edge plays fully on that side. Set `Range` to fade sounds out with distance (linear, silent at `Range`; 0 turns attenuation off):

```go
e.Listener().Range = 600
```

Pan is linear and keeps the center at full volume on both sides; panning only lowers the far side.

## SoundEmitter

`gfx.SoundEmitter` attaches a sound to an entity, and it is a Game Object component, so levels can place sounds (`gfx.SoundSource` is a `Transform` plus an emitter):

```json
{"group": "gfx", "class": "SoundSource", "components": {
  "gfx:Transform": {"Pos": {"X": 200, "Y": 40}},
  "gfx:SoundEmitter": {"Sound": "sfx/waterfall.ogg", "Volume": 0.6, "Loop": true}}}
```

| Field | Meaning |
| --- | --- |
| `Sound` | The effect, an asset key (`"kind": "sound"`) in data. A loading sound starts when ready; a failed one sets `Done`. |
| `Volume`, `Bus` | As for requests; zero volume means 1. |
| `Loop` | Off: play once. On: play while the emitter exists. |
| `Voice`, `Done` | Runtime only, never serialized. |

- **One-shot** (`Loop` off): starts once, sets `Done`, and plays to its end even if the entity is deleted in the next tick (an explosion entity can die with its sound still ringing). Clear `Done` to play it again.
- **Looping**: keeps one voice for as long as the entity has the component and `Done` is unset. Deleting the entity, removing the component or setting `Done` stops it.
- With a `gfx.Transform` the emitter is positional, and pan and distance follow the entity every tick.

## Streams

A stream source plays PCM that code produces while it plays (procedural audio, a synthesizer) instead of a decoded file. Give the `AudioSystem` an `io.Reader` that delivers stereo frames at the context's sample rate:

```go
st, err := audioSys.AddStream(sound.StreamSource{
	Reader: src,                   // read on the audio goroutine
	Format: sound.StreamFloat32,   // or sound.StreamInt16
	Bus:    sound.BusVoice,
	Volume: 1,                     // 0 means 1
})
defer st.Close()
```

- The stream's volume is bus × master × its own, applied every tick; `st.SetVolume` and `st.SetPan` change it live. Streams do not count against `Mixer.MaxVoices`. `AudioSystem.Close` closes them, and a stream whose reader returns an error ends by itself.
- `Reader` runs on the audio goroutine: it must be safe against the game goroutine and should neither block nor allocate. `Buffer` sets the player buffer (default 50 ms): smaller follows the source faster, larger survives slow frames.
- `st.Reading()` tells whether the audio side pulls samples: true after the first read, false once 200 ms pass without one (a browser context before the first click, a paused window) and always under `NullBackend`, which never reads. A source that queues commands for its next read uses it to drop what nobody heard.

## Tick order and costs

The audio stage is the last of the tick (after extract and sort), so every request of the tick starts in that tick, with the listener at the camera's final position. `AudioSystem.Run` allocates nothing in steady state (checked by `TestAudioSystemRunDoesNotAllocate`); starting a voice allocates an Ebiten player.

## Backends and headless runs

The `AudioSystem` hands voices to a `sound.Backend`:

| Backend | Plays |
| --- | --- |
| `sound.NewEbitenBackend(ctx)` | Through Ebiten's `audio.Context`. Each voice and stream reads through a gain stage, because Ebiten players have a volume but no pan; volume and pan change live through an atomic, without locks on the audio thread. |
| `sound.NullBackend{}` | Nothing. One-shots end at once, loops and streams last until stopped (a stream is never read). For servers, tools and tests. |

`game.Config{NoAudio: true}` uses the null backend and never creates the audio context. Ebiten allows **one** audio context per process; `game.New` reuses an existing one (and its sample rate) instead of panicking. Ebiten opens the device only once the game loop runs, so tests that call `Engine.Update` directly never touch a sound card either way.

## Without game.Engine

```go
ctx := audio.NewContext(sound.DefaultSampleRate)
sound.RegisterLoaders(srv, ctx.SampleRate())   // sounds decode at the device's rate

ecs.SetResource(w, &sound.Queue{})
ecs.SetResource(w, sound.NewMixer())
ecs.SetResource(w, &sound.Listener{HalfWidth: 320})  // optional; keep Pos at the camera

audioSys := sound.NewAudioSystem(w, sound.NewEbitenBackend(ctx))
s.AddStage("audio")
s.AddRunner(audioSys)   // last stage of Update; call audioSys.Close() at exit
```

The `resolver` package knows the kinds `sound` and `music`, so GOD data can name both.

## Not built yet

Loop points and intro sections for music, pitch, ducking (lowering music under dialogue), pausing audio when the window loses focus. Sounds and music reload with [asset hot reload](assets.md#hot-reload) like every asset.
