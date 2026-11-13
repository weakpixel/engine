// +game:group:name=speech
//
// Package speech lets entities speak with the voices of the
// github.com/weakpixel/voice library: the text of a gfx.Label types out in
// step with musical gibberish on the mixer's dialogue bus.
//
// Assets: [Language] (*.lang.json), [Voice] (*.voice.json, or a starter
// voice by name such as "chirpy"), [Sample] (a 16-bit PCM WAV that a voice's
// instrument plays) and [Dialogue] (*.dialogue.json). [RegisterLoaders]
// registers them on an asset server; [SetLibraryLoaders] maps the library
// kinds to them.
//
// Speaking: [Install] adds speech to a game.Engine and returns its [Speech].
// An entity with a [Speaker] component speaks with [Speech.Say]; with a
// gfx.Label, its text shows what was said so far. What happens to a line
// arrives as [Event] values through system.EventsOf. See
// doc/guide/engine/speech.md.
//
// It is the only package of the engine that imports the voice library, and it
// makes no Ebiten calls. Package game never imports speech: speech is opt-in.
package speech

//go:generate go run github.com/weakpixel/engine/tools/weakpixel gen
