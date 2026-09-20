/**
 * Architectural gates of the frontend source (P18-T03): components and
 * clients never call `fetch` directly, and nothing under `web/src` may import
 * a Node built-in, because that source is served to the browser as-is.
 */
import assert from "node:assert/strict";
import { globSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";

const webRoot = join(import.meta.dirname, "..", "..");
const HTTP_CORE = "src/core/http.ts";

/** Every browser-facing source file, sorted for stable output. */
function sources(): readonly string[] {
  return globSync("src/**/*.ts", { cwd: webRoot }).sort();
}

/** Reads one scanned file. */
function read(file: string): string {
  return readFileSync(join(webRoot, file), "utf8");
}

test("only the HTTP core calls fetch", () => {
  const files = sources();
  assert.ok(files.length >= 4, `expected to scan the frontend sources, found ${files.length}`);
  assert.ok(files.includes(HTTP_CORE), `expected ${HTTP_CORE} to be scanned`);

  const offenders = files.filter((file) => file !== HTTP_CORE && /\bfetch\s*\(/.test(read(file)));
  assert.deepEqual(offenders, [], "components and clients must go through the HTTP core");
});

test("the browser source imports no Node built-in", () => {
  const files = sources();
  const offenders = files.filter((file) => /from\s+["']node:/.test(read(file)));
  assert.deepEqual(offenders, []);
});
