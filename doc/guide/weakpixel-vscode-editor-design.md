# WeakPixel VS Code Extension – Editor Design Notes

Design suggestions for building a VS Code extension that supports both visual editing and code-first workflows for the WeakPixel engine (Go + Ebitengine).

---

## 1. Guiding principle: don't fight VS Code's window model

Map each engine tool to the native VS Code slot that already behaves the way users expect. Avoid one giant webview that tries to replicate a Unity-style dockable editor: it loses keyboard handling, theming, split views and layout memory, and never feels native.

**The scene file is the source of truth.** Visual edits and code edits both operate on the same diffable text file. This is what bridges the two workflows.

---

## 2. Mapping engine tools to VS Code slots

| Engine tool | VS Code slot | Why |
|---|---|---|
| Scene / prefab files | **Custom Editor** (`CustomTextEditorProvider`) | Dirty state, save, undo/redo, git diff, split view and floating windows for free. "Reopen With… Text Editor" for code-first users. |
| Viewport | Inside the custom editor's **webview** | Ebitengine compiles to WASM and runs in the webview. |
| Hierarchy | Native **TreeView** in your own view container | Native keyboard navigation, filtering and theming. Use `TreeDragAndDropController` for reparenting. |
| Inspector | **WebviewView**, ideally in the secondary (right) sidebar | Property grids don't exist natively, so this is the one place custom UI is justified. |
| Asset browser | **Explorer** for structure + **thumbnail grid** in the bottom panel (see §6) | Reuse what exists; use the panel's width for visual browsing. |
| Play mode / debugging | **Debug Adapter (DAP)** | Breakpoints, debug console and run/stop buttons for free. |

### Inspector styling
- Use VS Code's CSS theme variables (`--vscode-*`) so it matches every theme.
- The official webview-ui-toolkit is deprecated. Use something like **vscode-elements** or your own small component set.

---

## 3. Making separate panels feel like one tool

- **One "editor session" service in the extension host** owns the selection and the active scene. Every view (viewport, tree, inspector) subscribes to it. Clicking in the viewport selects in the tree and fills the inspector.
- **Engine communication over JSON-RPC**, similar to a language server. Keeps the extension thin and the engine independent.
- **Plan for webviews being destroyed.** Moving webviews between editor groups or hiding them can reload them. Serialize their state, and use `retainContextWhenHidden` only for the viewport because it costs memory.
- **You can't fully dictate layout.** Provide sensible default view locations, plus a **"WeakPixel: Reset Editor Layout"** command that reopens and focuses all views. VS Code remembers any arrangement the user makes after that.
- **Floating windows:** users can drag the scene editor into its own window (e.g. a second monitor), which works because it is a real editor rather than a panel.

---

## 4. Viewport architecture (Ebitengine → WASM in webview)

Use **one engine build in two modes** inside the custom editor webview.

### Edit mode
- The WASM build loads the scene file; game logic is paused.
- The engine handles picking, gizmos and the editor camera.
- Clicks send entity IDs out: `syscall/js` → `postMessage` → extension's session service → the tree and inspector update.

### Play mode
- The same WASM build runs game logic.
- On stop, revert to the file's state (as Unity does), so play-time changes never leak into the scene.

### One-way edit flow
```
Inspector / Tree change
   → extension applies a WorkspaceEdit to the scene document
   → the document change is pushed to the engine
```
Undo/redo and dirty state then come from VS Code instead of a custom system.

### Gizmos
Gizmos are editor-only on-screen handles for manipulating objects directly:
- **Transform:** move arrows (X/Y), rotation ring, scale handles.
- **Bounds and colliders:** outlines of collision shapes and bounding boxes.
- **Helpers for invisible objects:** icons for cameras, sound sources, spawn points and triggers, plus camera bounds.
- **Guides:** snapping grid and selection highlight.

Draw gizmos **in Ebitengine on an editor-only layer** rather than as an HTML overlay. The engine already knows world positions and the camera transform, so rendering and hit-testing use the same math.

---

## 5. Hot reload

- Before swapping the WASM module, **snapshot the runtime scene state** to the extension host, then restore it afterwards. Users keep the camera position, selection and play state across saves.
- Go → WASM build times grow with project size:
  - Debounce rebuilds.
  - Show build status in the status bar.
  - Keep editor-only code in a separate package so it doesn't rebuild on every gameplay change.

---

## 6. Asset browser

A bottom panel is a poor fit for trees and lists but a good fit for a **thumbnail grid**. Split the job three ways:

### Structure → reuse the Explorer
Assets are already workspace files, so the Explorer is a native tree-shaped asset browser. Enhance it:
- A file icon theme or `FileDecorationProvider` for asset types and badges (e.g. "unused", "missing reference").
- Context-menu commands: "Create Prefab", "Create Material", "Open in Scene".
- Drag and drop from the Explorer into the viewport. **Note:** in webviews, VS Code requires holding **Shift** while dropping files. Document this, or users will think drop is broken.

### Visual picking → thumbnail grid in the bottom panel
- Sprites, tilesets and animations side by side.
- Filter by the folder selected in the Explorer, or by type ("all sprites").

### Finding things → Quick Pick
- A fuzzy **"Go to Asset"** command with type filters.

---

## 7. Code-first bridge (Go tooling)

- Ship a small **Go CLI** that uses `go/packages` to scan component structs and emit a **JSON schema**.
- Use **struct tags** to drive inspector widgets, e.g.:
  ```go
  type Health struct {
      Max     int     `editor:"range=1,1000"`
      Regen   float64 `editor:"range=0,1"`
      Tint    Color   `editor:"color"`
  }
  ```
- Rerun the scan on save. This works like reflection with no runtime cost.
- Use the same scan for:
  - **CodeLens** on component structs ("Used in 3 scenes", "Show in Inspector").
  - Jumping from an inspector field to its Go definition through **gopls**.
- Hot reload on save closes the loop between code and the visual editor.

---

## 8. Debugging

- Delve can't debug WASM inside a webview.
- Add a launch configuration that runs a **native desktop build under `dlv dap`** against the current scene.
- The **webview is for fast iteration** and the **native window is for breakpoints**. Users get both without the tool feeling split.

---

## 9. Input focus gotcha

When the viewport has focus, the webview swallows keystrokes, which conflicts with VS Code shortcuts (Ctrl+S, Ctrl+P, …).
- Forward unhandled keys back to VS Code.
- Capture input only while play mode is active or the mouse is over the canvas.

---

## 10. Summary checklist

- [ ] Text-based, diffable scene format as the single source of truth
- [ ] `CustomTextEditorProvider` for scenes/prefabs with an embedded Ebitengine WASM viewport
- [ ] Edit mode / play mode in the same WASM build; revert on stop
- [ ] Native TreeView hierarchy with drag-and-drop reparenting
- [ ] WebviewView inspector (secondary sidebar) driven by the JSON schema from struct tags
- [ ] Central editor session service for shared selection
- [ ] Edits via `WorkspaceEdit` for native undo/redo
- [ ] Gizmos on an editor-only Ebitengine layer
- [ ] Hot reload with state snapshot/restore, debounced builds and status bar
- [ ] Explorer enhancements + thumbnail grid panel + "Go to Asset" quick pick
- [ ] Go schema CLI + CodeLens + gopls navigation
- [ ] Native `dlv dap` launch config for debugging
- [ ] Keyboard forwarding from the viewport webview
- [ ] "Reset Editor Layout" command
