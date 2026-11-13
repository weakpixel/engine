<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  --bg: #fff; --fg: #1f2328; --muted: #656d76; --border: #d0d7de; --panel: #f6f8fa;
  --accent: #0969da; --badge-bg: #ddf4ff; --badge-fg: #0550ae;
  --embed-bg: #fff8c5; --embed-fg: #7d4e00;
}
/* Dark follows the browser unless the user picked light; an explicit choice
   (data-theme, set by the toggle) always wins. */
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0; --border: #30363d; --panel: #161b22;
    --accent: #58a6ff; --badge-bg: #10243e; --badge-fg: #79c0ff;
    --embed-bg: #3b2e00; --embed-fg: #e3b341;
  }
}
:root[data-theme="dark"] {
  --bg: #0d1117; --fg: #e6edf3; --muted: #8d96a0; --border: #30363d; --panel: #161b22;
  --accent: #58a6ff; --badge-bg: #10243e; --badge-fg: #79c0ff;
  --embed-bg: #3b2e00; --embed-fg: #e3b341;
}
:root { color-scheme: light dark; }
:root[data-theme="light"] { color-scheme: light; }
:root[data-theme="dark"] { color-scheme: dark; }
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--fg); font: 15px/1.5 system-ui, sans-serif; display: flex; }
a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }
nav { position: sticky; top: 0; align-self: flex-start; width: 280px; height: 100vh; overflow-y: auto;
  padding: 16px; border-right: 1px solid var(--border); background: var(--panel); flex-shrink: 0; }
nav h1 { font-size: 17px; margin: 0 0 12px; }
nav input { width: 100%; padding: 6px 8px; margin-bottom: 12px; color: inherit; background: var(--bg);
  border: 1px solid var(--border); border-radius: 6px; }
.theme { display: none; float: right; padding: 2px 8px; cursor: pointer; color: inherit; background: var(--bg);
  border: 1px solid var(--border); border-radius: 6px; font-size: 13px; }
.js .theme { display: block; }
.tabs { display: none; gap: 4px; margin-bottom: 8px; }
.js .tabs { display: flex; }
.tabs button { flex: 1; padding: 4px; cursor: pointer; color: inherit; background: var(--bg);
  border: 1px solid var(--border); border-radius: 6px; }
.tabs button[aria-selected="true"] { background: var(--accent); color: #fff; border-color: var(--accent); }
nav h2 { font-size: 12px; text-transform: uppercase; color: var(--muted); margin: 16px 0 4px; }
nav ul { list-style: none; margin: 0; padding: 0; }
nav li { padding: 1px 0; }
nav ul ul { padding-left: 12px; }
[hidden] { display: none !important; }
main { flex: 1; min-width: 0; max-width: 900px; padding: 8px 32px 64px; }
section { padding-top: 24px; }
h2.part { border-bottom: 1px solid var(--border); padding-bottom: 4px; margin-top: 40px; }
h3 { margin: 0 0 4px; }
p { margin: 8px 0; white-space: pre-line; }
table { border-collapse: collapse; width: 100%; margin: 8px 0; font-size: 14px; }
th, td { border: 1px solid var(--border); padding: 4px 8px; text-align: left; vertical-align: top; }
th { background: var(--panel); }
code { font: 13px ui-monospace, monospace; }
ul.values { margin: 4px 0 0; padding-left: 18px; }
.badge { display: inline-block; padding: 0 6px; margin-right: 4px; border-radius: 10px; font-size: 12px;
  background: var(--badge-bg); color: var(--badge-fg); }
.badge.embedded { background: var(--embed-bg); color: var(--embed-fg); }
details { margin: 8px 0; }
summary { cursor: pointer; color: var(--muted); }
.used { color: var(--muted); font-size: 14px; }
@media (max-width: 760px) {
  body { display: block; }
  nav { position: static; width: auto; height: auto; border-right: 0; border-bottom: 1px solid var(--border); }
  main { padding: 8px 16px 48px; }
}
</style>
<script>
/* Apply a saved choice before first paint; without one the browser default applies. */
try { var t = localStorage.getItem("god-doc-theme"); if (t === "light" || t === "dark") document.documentElement.dataset.theme = t; } catch (e) {}
</script>
</head>
<body>
<nav>
<button type="button" class="theme" id="theme" aria-label="Switch between light and dark mode"></button>
<h1>{{.Title}}</h1>
<input type="search" id="filter" placeholder="Filter objects and components" aria-label="Filter">
<div class="tabs" role="tablist">
<button type="button" role="tab" data-tab="objects" aria-selected="true">Objects</button>
<button type="button" role="tab" data-tab="components" aria-selected="false">Components</button>
</div>
<div id="tab-objects">
<h2>Objects</h2>
<ul>
{{- range .Groups}}
<li>{{.Name}}
<ul>
{{- range .Objects}}
<li data-filter="{{.Key}}"><a href="#{{.ID}}">{{.Class}}</a></li>
{{- end}}
</ul></li>
{{- end}}
</ul>
</div>
<div id="tab-components">
<h2>Components</h2>
<ul>
{{- range .Components}}
<li data-filter="{{.Key}}"><a href="#{{.ID}}">{{.Key}}</a></li>
{{- end}}
</ul>
</div>
</nav>
<main>
<h2 class="part">Objects</h2>
{{- range .Groups}}{{range .Objects}}
<section id="{{.ID}}" data-filter="{{.Key}}">
<h3>{{.Key}}</h3>
{{- range .Doc}}
<p>{{.}}</p>
{{- end}}
{{- if .Components}}
<table>
<thead><tr><th>Component</th><th>Field</th><th>Flags</th><th>Description</th></tr></thead>
<tbody>
{{- range .Components}}
<tr><td><a href="#{{.CompID}}">{{.Key}}</a></td><td><code>{{.Field}}</code></td>
<td>{{if .Optional}}<span class="badge">optional</span>{{end}}{{if .Readonly}}<span class="badge">readonly</span>{{end}}{{if .Embedded}}<span class="badge embedded">{{.Embedded}}</span>{{end}}</td>
<td>{{.Doc}}</td></tr>
{{- end}}
</tbody>
</table>
{{- else}}
<p><em>No components.</em></p>
{{- end}}
</section>
{{- end}}{{end}}
<h2 class="part">Components</h2>
{{- range .Components}}
<section id="{{.ID}}" data-filter="{{.Key}}">
<h3>{{.Key}}</h3>
{{- range .Doc}}
<p>{{.}}</p>
{{- end}}
{{- if .GoType}}
<p>Go type: <code>{{.GoType}}</code></p>
{{- end}}
{{- if .Rows}}
<details open>
<summary>Fields</summary>
<table>
<thead><tr><th>Field</th><th>Type</th><th>Required</th><th>Nullable</th><th>Description</th></tr></thead>
<tbody>
{{- range .Rows}}
<tr><td><code>{{.Name}}</code></td><td><code>{{.Type}}</code>{{if .Runtime}} <em>(runtime)</em>{{end}}</td><td>{{if .Required}}yes{{end}}</td><td>{{if .Nullable}}yes{{end}}</td><td>{{.Description}}{{if .Values}}<ul class="values">{{range .Values}}<li><code>{{.Value}} {{.Name}}</code>{{if .Doc}}: {{.Doc}}{{end}}</li>{{end}}</ul>{{end}}{{if .Default}} <span class="default">Default: <code>{{.Default}}</code></span>{{end}}</td></tr>
{{- end}}
</tbody>
</table>
</details>
{{- else}}
<p>Type: <code>{{.Type}}</code></p>
{{- end}}
<p class="used">Used by:
{{- range .Users}} <a href="#{{.ID}}">{{.Key}}</a>{{if .Embedded}} <span class="badge embedded">{{.Embedded}}</span>{{end}}{{else}} <em>no object yet</em>{{end}}</p>
</section>
{{- end}}
</main>
<script>
(function () {
  document.documentElement.classList.add("js");
  var root = document.documentElement, themeBtn = document.getElementById("theme");
  var query = window.matchMedia("(prefers-color-scheme: dark)");
  function effective() { return root.dataset.theme || (query.matches ? "dark" : "light"); }
  function label() { themeBtn.textContent = effective() === "dark" ? "Light mode" : "Dark mode"; }
  themeBtn.addEventListener("click", function () {
    var next = effective() === "dark" ? "light" : "dark";
    root.dataset.theme = next;
    try { localStorage.setItem("god-doc-theme", next); } catch (e) {}
    label();
  });
  query.addEventListener("change", label);
  label();
  var tabs = document.querySelectorAll("[data-tab]");
  function show(name) {
    tabs.forEach(function (b) { b.setAttribute("aria-selected", String(b.dataset.tab === name)); });
    document.getElementById("tab-objects").hidden = name !== "objects";
    document.getElementById("tab-components").hidden = name !== "components";
  }
  tabs.forEach(function (b) { b.addEventListener("click", function () { show(b.dataset.tab); }); });
  show("objects");
  document.getElementById("filter").addEventListener("input", function (e) {
    var q = e.target.value.toLowerCase();
    document.querySelectorAll("[data-filter]").forEach(function (el) {
      el.hidden = q !== "" && el.dataset.filter.toLowerCase().indexOf(q) < 0;
    });
  });
})();
</script>
</body>
</html>
