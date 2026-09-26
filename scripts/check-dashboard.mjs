// Verifies the embedded dashboard artifact is the real Vite build, not the
// fallback placeholder. Reads dist/index.html directly (no server needed) and
// fails if the app root, a hashed JS asset, or the theme attribute is missing.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const dist = join(root, "internal/server/dashboard/dist");
const html = readFileSync(join(dist, "index.html"), "utf8");

const checks = [
  ["app root", html.includes('id="root"')],
  ["theme attribute", html.includes('data-theme="calcron"')],
  ["hashed script asset", /\/assets\/index-[A-Za-z0-9_-]+\.js/.test(html)],
  ["stylesheet asset", /\/assets\/index-[A-Za-z0-9_-]+\.css/.test(html)],
  ["no placeholder", !html.includes("not built")],
];

const assets = readdirSync(join(dist, "assets"));
const js = assets.filter((f) => f.endsWith(".js"));
const css = assets.filter((f) => f.endsWith(".css"));
checks.push(["js bundle exists", js.length > 0 && statSync(join(dist, "assets", js[0])).size > 10_000]);
checks.push(["css bundle exists", css.length > 0 && statSync(join(dist, "assets", css[0])).size > 10_000]);

const failed = checks.filter(([, ok]) => !ok);
for (const [name, ok] of checks) console.log(`${ok ? "ok" : "FAIL"}  ${name}`);
if (failed.length) {
  console.error(`dashboard build failed: ${failed.map(([n]) => n).join(", ")}`);
  process.exit(1);
}
console.log("dashboard build verified");
