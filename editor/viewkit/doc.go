// Package viewkit holds what the wpix-editor tools that show a texture
// share: the texture sprite kept on screen ([Texture]), the frames over it
// with hover, selection, labels and a list to pick from ([Frames]), and the
// overlay text in window pixels ([Labels]). The previewers of package viewer
// and the sheet tool use it, so a sheet and an atlas draw their frames the
// same way. See doc/design/tool-shell.md#tools.
package viewkit
