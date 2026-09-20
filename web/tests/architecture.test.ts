/**
 * Architectural gates of the frontend source (P18-T03, extended in P18-T04).
 *
 * Everything under `web/src` is served to the browser as-is, so these tests
 * read the sources and refuse: a `fetch` call outside the HTTP core, a Node
 * built-in import, unsafe rendering (`innerHTML` and friends), a UI component
 * that reaches for the translation singleton instead of receiving its text,
 * and any runtime dependency in `web/package.json`.
 */
import assert from "node:assert/strict";
import { test } from "node:test";

import { browserSources, readPackageFile } from "./support/paths.js";

/** The single module allowed to touch the network. */
const HTTP_CORE = "src/core/http.ts";

/** Property accesses and calls banned for dynamic data. */
const UNSAFE_RENDERING: readonly RegExp[] = [
  /\.innerHTML/,
  /\.outerHTML/,
  /insertAdjacentHTML\s*\(/,
  /\beval\s*\(/,
];

test("only the HTTP core calls fetch", () => {
  const files = browserSources();
  assert.ok(files.length >= 8, `expected to scan the frontend sources, found ${files.length}`);
  assert.ok(files.includes(HTTP_CORE), `expected ${HTTP_CORE} to be scanned`);

  const offenders = files.filter((file) => file !== HTTP_CORE && /\bfetch\s*\(/.test(readPackageFile(file)));
  assert.deepEqual(offenders, [], "components and clients must go through the HTTP core");
});

test("the browser source imports no Node built-in", () => {
  const offenders = browserSources().filter((file) => /from\s+["']node:/.test(readPackageFile(file)));
  assert.deepEqual(offenders, []);
});

test("the browser source renders dynamic data without innerHTML", () => {
  const offenders: string[] = [];
  for (const file of browserSources()) {
    const source = readPackageFile(file);
    for (const pattern of UNSAFE_RENDERING) {
      if (pattern.test(source)) {
        offenders.push(`${file} matches ${String(pattern)}`);
      }
    }
  }
  assert.deepEqual(offenders, [], "dynamic data reaches the DOM through textContent or text nodes only");
});

test("components do not import the translation singleton", () => {
  const components = browserSources().filter((file) => file.startsWith("src/components/"));
  assert.ok(components.length >= 5, `expected to scan the components, found ${components.length}`);

  const offenders = components.filter((file) => /i18n\/generated/.test(readPackageFile(file)));
  assert.deepEqual(offenders, [], "components receive their translated text, never a translation module");
});

test("the browser bundle carries no runtime dependency", () => {
  const manifest = JSON.parse(readPackageFile("package.json")) as {
    readonly dependencies?: Readonly<Record<string, string>>;
    readonly devDependencies?: Readonly<Record<string, string>>;
  };

  assert.deepEqual(Object.keys(manifest.dependencies ?? {}), [], "runtime dependencies must stay empty");
  assert.deepEqual(
    Object.keys(manifest.devDependencies ?? {}).sort(),
    ["typescript"],
    "the build dependency is the official TypeScript package only",
  );
});
