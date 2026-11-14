package speech

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/asset/library"
)

// SetLibraryLoaders maps the library kinds of the speech files
// ([library.KindVoice], [library.KindDialogue], [library.KindLanguage]) to
// their load functions, so a library group loads the voices, dialogues and
// languages in its folder. [RegisterLoaders] must have run on the server the
// library is attached to.
//
// game cannot set these itself (speech is installed on an engine, so
// game never imports it); a game that uses a library calls this once, or
// lets speech.Install do it.
func SetLibraryLoaders(lib *library.Library) {
	lib.SetLoader(library.KindVoice, func(sc *asset.Scope, key string) { LoadVoice(sc, key) })
	lib.SetLoader(library.KindDialogue, func(sc *asset.Scope, key string) { LoadDialogue(sc, key) })
	lib.SetLoader(library.KindLanguage, func(sc *asset.Scope, key string) { LoadLanguage(sc, key) })
}
