// Package host is the entry point of a game's editor build. A game that
// wants the level editor describes itself with a [Game] and calls [Run] from
// a main that only exists under the editor build tag; the host then does
// what every game used to wire by hand: it connects the VSCode extension's
// bridge (or falls back to a plain editor session), reads assets from the
// workspace, creates the engine and opens the file in the stage its kind
// asks for.
//
// There are two stages. The level stage runs the game's systems (Game.Setup)
// around the edited level. The prefab stage runs none of them: it shows the
// prefab alone on a grid ([editor.Stage]). The extension names the stage and
// the first file on the host page; the page is reloaded when the stage
// changes, because Ebiten runs one game per process.
//
// See doc/guide/engine/editor.md.
package host
