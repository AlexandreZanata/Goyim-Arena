/**
 * Tests of the page localization (P18-T09).
 *
 * What is proven here is that a language change is a change of the translator
 * the page holds — the listeners re-render, nothing reloads the document — and
 * that the value is per page: two localizations in one process never share a
 * language, which is exactly what a module-level singleton would break.
 */
import assert from "node:assert/strict";
import { test } from "node:test";

import { createLocalization } from "../../src/i18n/localization.js";
import type { Locale } from "../../src/i18n/locale.js";
import type { Translator } from "../../src/i18n/translator.js";

test("switching language re-renders what the page rendered, without a reload", () => {
  const localization = createLocalization("pt-BR");
  const rendered: string[] = [];
  localization.subscribe((translator: Translator, previous: Locale) => {
    rendered.push(`${previous}->${translator.locale}:${translator.translate("auth.login.submit")}`);
  });

  const before = localization.translator;
  localization.switchTo("en-US");

  assert.equal(localization.locale, "en-US");
  assert.notEqual(localization.translator, before, "a switch hands out a translator of the new locale");
  assert.equal(before.translate("auth.login.submit"), "Entrar", "the translator already used keeps its locale");
  assert.deepEqual(rendered, ["pt-BR->en-US:Sign in"]);
});

test("switching to the current locale changes nothing and tells nobody", () => {
  const localization = createLocalization("pt-BR");
  let notifications = 0;
  localization.subscribe(() => {
    notifications += 1;
  });

  const translator = localization.translator;
  assert.equal(localization.switchTo("pt-BR"), translator);
  assert.equal(notifications, 0);

  localization.switchTo("en-US");
  assert.equal(notifications, 1);
});

test("a listener stops being told once it unsubscribes", () => {
  const localization = createLocalization("pt-BR");
  let notifications = 0;
  const unsubscribe = localization.subscribe(() => {
    notifications += 1;
  });

  unsubscribe();
  localization.switchTo("en-US");
  assert.equal(notifications, 0);
  assert.equal(localization.locale, "en-US", "the switch still happened");
});

test("a listener that unsubscribes while being notified does not disturb the pass", () => {
  const localization = createLocalization("pt-BR");
  const seen: string[] = [];
  let unsubscribe: () => void = () => {};
  unsubscribe = localization.subscribe(() => {
    seen.push("first");
    unsubscribe();
  });
  localization.subscribe(() => {
    seen.push("second");
  });

  localization.switchTo("en-US");
  assert.deepEqual([...seen].sort(), ["first", "second"]);
});

test("two pages of one process keep their own language", () => {
  const first = createLocalization("pt-BR");
  const second = createLocalization("pt-BR");

  first.switchTo("en-US");

  assert.equal(first.locale, "en-US");
  assert.equal(second.locale, "pt-BR", "a page must not inherit the language of another");
  assert.equal(second.translator.translate("auth.login.submit"), "Entrar");
});

test("a locale the product does not ship is refused, never reflected", () => {
  const localization = createLocalization("pt-BR");

  assert.throws(() => localization.switchTo("pt-PT" as Locale), TypeError);
  assert.throws(() => localization.switchTo("de-DE" as Locale), TypeError);
  assert.throws(() => createLocalization("pt-br" as Locale), TypeError);
  assert.throws(() => createLocalization(undefined as unknown as Locale), TypeError);
  assert.equal(localization.locale, "pt-BR", "a refused switch leaves the page where it was");
});
