// Package hostlink is what a game or tool that runs under an editor host
// shares with that host, whichever protocol it speaks: the protocol version,
// the transport interface, the connection to the host page of a browser
// build, and the settings the host passes (stage, tool, first file, asset
// root, file index). In a browser the settings come from
// globalThis.weakpixelHost, which the VSCode webview defines before the game
// starts; natively they come from WEAKPIXEL_* environment variables.
//
// Both the level editor's bridge (editor/bridge) and the tool shell
// (editor/toolshell, tools/wpix-editor) build on it. It imports
// only the standard library, so a tool links neither the bridge nor the
// level editor. See doc/design/editor.md#protocol.
package hostlink
