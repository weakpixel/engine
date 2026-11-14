// Smoke test of browser builds (a game's make wasm-smoke runs it):
// serves each build from <dist>/<name>/, loads it in headless Chromium,
// clicks the start overlay, presses a key, lets it run, and fails on a Go
// panic, a fatal error or an uncaught page error. A build whose name ends in
// "-packed" (a build.sh -packed build) must also fetch its assets only as the
// catalog and packs under assets/, and at least one pack. Needs Node and
// Playwright; make skips it when they are missing.
//
//   node smoke.cjs <dist> <name> ...
"use strict";
const http = require("http");
const fs = require("fs");
const path = require("path");
const { chromium } = require("playwright");

const runMillis = 5000;
const types = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm", ".json": "application/json", ".zip": "application/zip" };

// serve starts a static server for dir on a free port; requested collects
// the paths it was asked for.
function serve(dir, requested) {
  const server = http.createServer((req, res) => {
    const name = decodeURIComponent(new URL(req.url, "http://x").pathname);
    requested.push(name);
    const file = path.join(dir, name === "/" ? "index.html" : name);
    if (!file.startsWith(dir) || !fs.existsSync(file)) {
      res.writeHead(404).end();
      return;
    }
    res.writeHead(200, { "Content-Type": types[path.extname(file)] || "application/octet-stream" });
    fs.createReadStream(file).pipe(res);
  });
  return new Promise((resolve) => server.listen(0, "127.0.0.1", () => resolve(server)));
}

// packedProblems checks the asset requests of a packed build: only the
// catalog and packs, and at least one pack.
function packedProblems(requested) {
  const assets = requested.filter((p) => p.startsWith("/assets/"));
  const loose = assets.filter((p) => p !== "/assets/assets.catalog.json" && !/^\/assets\/packs\/[^/]+\.zip$/.test(p));
  const problems = loose.map((p) => `fetched ${p} outside a pack`);
  if (!assets.some((p) => p.startsWith("/assets/packs/"))) problems.push("fetched no pack");
  return problems;
}

async function runDemo(browser, dir, packed) {
  const requested = [];
  const server = await serve(dir, requested);
  const page = await browser.newPage({ viewport: { width: 960, height: 540 } });
  const problems = [];
  let inPanic = false;
  page.on("console", (msg) => {
    const text = msg.text();
    // A pack that fails to mount, or a key read outside its mounted pack.
    if (packed && text.includes("library: ")) problems.push(text);
    if (/^(panic: |fatal error: |exit code: [1-9])/.test(text)) inPanic = true;
    if (inPanic) problems.push(text);
  });
  page.on("pageerror", (err) => problems.push(`page error: ${err.message}`));
  try {
    await page.goto(`http://127.0.0.1:${server.address().port}/`);
    await page.waitForFunction(() => document.getElementById("status").textContent === "Click to start"
      || document.getElementById("error").textContent !== "", null, { timeout: 60000 });
    await page.mouse.click(480, 270);
    await page.keyboard.press("ArrowRight");
    await page.waitForTimeout(runMillis);
    const shown = await page.evaluate(() => document.getElementById("error").textContent);
    if (shown && problems.length === 0) problems.push(shown);
    if (packed) problems.push(...packedProblems(requested));
  } finally {
    await page.close();
    server.close();
  }
  return problems;
}

(async () => {
  const [root, ...names] = process.argv.slice(2);
  const browser = await chromium.launch({ args: ["--enable-unsafe-swiftshader", "--use-angle=swiftshader"] });
  let failed = 0;
  for (const name of names) {
    const problems = await runDemo(browser, path.resolve(root, name), name.endsWith("-packed"));
    if (problems.length) {
      failed++;
      console.log(`FAIL ${name}\n  ${problems.slice(0, 20).join("\n  ")}`);
    } else {
      console.log(`ok   ${name}`);
    }
  }
  await browser.close();
  process.exit(failed ? 1 : 0);
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
