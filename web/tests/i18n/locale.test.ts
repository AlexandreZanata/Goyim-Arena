/**
 * Tests of the interface locale identity (P18-T09).
 *
 * The product ships the catalogs it ships, so the whole question here is what
 * happens to a value that is *not* one of them: a near miss ("pt-PT" for a
 * product with "pt-BR"), a non-canonical spelling, a malformed tag or a value
 * that is not even a string. None of them may reach a page.
 */
import assert from "node:assert/strict";
import { test } from "node:test";

import { messages } from "../../src/i18n/generated.js";
import {
  canonicalLocale,
  canonicalTag,
  defaultLocale,
  isSupportedLocale,
  resolveLocale,
  supportedLocales,
} from "../../src/i18n/locale.js";

test("the shipped locales are exactly the locales of the generated catalog", () => {
  assert.deepEqual([...supportedLocales].sort(), Object.keys(messages).sort());
  assert.ok(supportedLocales.includes(defaultLocale), "the default locale must be one the product ships");
});

test("a well-formed tag is canonicalised through ICU", () => {
  assert.equal(canonicalTag("pt-br"), "pt-BR");
  assert.equal(canonicalTag("  en-US  "), "en-US");
  assert.equal(canonicalTag(""), null);
  assert.equal(canonicalTag("   "), null);
  assert.equal(canonicalTag(7), null);
  assert.equal(canonicalTag(null), null);
});

test("a malformed tag is refused instead of guessed", () => {
  for (const candidate of ["not a tag", "pt-BR<script>", "pt_BR", '../../etc/passwd', "pt-BR\"><img"]) {
    assert.equal(canonicalTag(candidate), null, `${candidate} must not be a language tag`);
    assert.equal(canonicalLocale(candidate), null, `${candidate} must not name a supported locale`);
  }
});

test("a supported locale is named by any spelling of it, and a near miss is not", () => {
  assert.equal(canonicalLocale("pt-br"), "pt-BR");
  assert.equal(canonicalLocale("EN-us"), "en-US");
  // The product ships pt-BR and en-US; "pt" and "pt-PT" are different locales
  // and serving them from pt-BR would be a translation decision nobody made.
  assert.equal(canonicalLocale("pt"), null);
  assert.equal(canonicalLocale("pt-PT"), null);
  assert.equal(canonicalLocale("en-GB"), null);
});

test("isSupportedLocale normalises nothing, so it is safe as a type guard", () => {
  assert.ok(isSupportedLocale("pt-BR"));
  assert.ok(isSupportedLocale("en-US"));
  assert.ok(!isSupportedLocale("pt-br"));
  assert.ok(!isSupportedLocale("pt-PT"));
  assert.ok(!isSupportedLocale(undefined));
});

test("resolution answers only with a shipped locale", () => {
  assert.equal(resolveLocale(["pt-PT", "de-DE", "pt-br"]), "pt-BR");
  assert.equal(resolveLocale([null, undefined, "", "en-us"]), "en-US");
  assert.equal(resolveLocale(["pt-PT", "en-GB"]), defaultLocale);
  assert.equal(resolveLocale([]), defaultLocale);
});
