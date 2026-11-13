# Toolbar icons

All editors of `weakpixel-editor` share these icons; a tool never keeps icons of its own.

The SVG files are icons from [Tabler Icons](https://github.com/tabler/tabler-icons)
(outline set, the `main` branch on 2026-10-21; the icon version is in each file's header),
MIT License, Copyright (c) 2020-2026 Paweł Kuna; the license text is in `LICENSE`.
The PNG files are those SVGs rendered white on a transparent background at 64x64 pixels
(headless Chromium, stroke set to white); the toolbar tints them.

| PNG | Tabler SVG | Used by |
| --- | --- | --- |
| `select` | `pointer` | anim |
| `pivot` | `focus-2` | anim |
| `hit` | `sword` | anim |
| `hurt` | `shield` | anim |
| `body` | `square-dashed` | anim |
| `move` | `arrows-move` | fx (put the emitter back at the origin) |
| `burst` | `sparkles` | fx (burst) |
| `play` | `player-play` | fx |
| `pause` | `player-pause` | fx |
| `step` | `player-track-next` | fx (step one tick) |
| `restart` | `refresh` | fx |
