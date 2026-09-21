/**
 * Tests of the native formatting helpers (P18-T09).
 *
 * The assertions split in two kinds, on purpose:
 *
 *   - where ICU itself can represent the value, the helper is compared against
 *     the rendering ICU produces for that same value. An assertion like that is
 *     about the helper, not about a formatting convention: it fails if the
 *     helper reimplements a separator, moves a symbol or drops grouping;
 *   - where ICU cannot represent the value exactly — an amount in minor units
 *     beyond the safe integer range — the assertion is on the digits, because
 *     that is what an implementation passing the amount through a float would
 *     get wrong.
 */
import assert from "node:assert/strict";
import { test } from "node:test";

import {
  currencyFractionDigits,
  formatCurrencyMinor,
  formatInstant,
  formatList,
  formatNumber,
  formatRelativeTime,
  instantOf,
  pluralCategory,
} from "../../src/i18n/formats.js";

/** The rendering ICU produces for one numeric amount. */
function icuAmount(locale: "pt-BR" | "en-US", amount: number, currency: string): string {
  return new Intl.NumberFormat(locale, { style: "currency", currency }).format(amount);
}

test("numbers are grouped and separated the way the locale does it", () => {
  assert.equal(formatNumber("pt-BR", 1234567.891), "1.234.567,891");
  assert.equal(formatNumber("en-US", 1234567.891), "1,234,567.891");
  assert.equal(formatNumber("pt-BR", 1234), "1.234");
  assert.equal(formatNumber("en-US", 1234), "1,234");
});

test("money renders exactly as the locale renders the same amount", () => {
  assert.equal(formatCurrencyMinor("pt-BR", 123456, "BRL"), icuAmount("pt-BR", 1234.56, "BRL"));
  assert.equal(formatCurrencyMinor("en-US", 123456, "USD"), icuAmount("en-US", 1234.56, "USD"));
  assert.equal(formatCurrencyMinor("pt-BR", -123456, "BRL"), icuAmount("pt-BR", -1234.56, "BRL"));
  assert.equal(formatCurrencyMinor("pt-BR", 5, "BRL"), icuAmount("pt-BR", 0.05, "BRL"));
  assert.equal(formatCurrencyMinor("en-US", 0, "USD"), icuAmount("en-US", 0, "USD"));
});

test("the scale of a currency comes from the locale, not from a constant", () => {
  assert.equal(currencyFractionDigits("pt-BR", "BRL"), 2);
  assert.equal(currencyFractionDigits("en-US", "USD"), 2);
  // A currency with no minor unit at all, and one with three.
  assert.equal(currencyFractionDigits("en-US", "JPY"), 0);
  assert.equal(currencyFractionDigits("en-US", "BHD"), 3);
  assert.equal(formatCurrencyMinor("en-US", 1234, "JPY"), icuAmount("en-US", 1234, "JPY"));
  assert.equal(formatCurrencyMinor("en-US", 1234, "BHD"), icuAmount("en-US", 1.234, "BHD"));
});

test("an amount beyond the safe integer range is exact, not rounded", () => {
  // 9_007_199_254_740_993 minor units is 90_071_992_547_409.93 in BRL. Read as
  // a float it would end in ...,92 or ...,94 depending on the rounding; the
  // integer assembly has no such freedom.
  const rendered = formatCurrencyMinor("pt-BR", 9_007_199_254_740_993n, "BRL");
  assert.ok(rendered.endsWith(",93"), `expected the exact cents, got ${rendered}`);
  assert.ok(rendered.includes("90.071.992.547.409"), `expected the grouped whole units, got ${rendered}`);
});

test("an amount that is not an exact number of minor units is refused", () => {
  assert.throws(() => formatCurrencyMinor("pt-BR", 12.5, "BRL"), TypeError);
  assert.throws(() => formatCurrencyMinor("en-US", 2 ** 53 + 2, "USD"), TypeError);
});

test("an instant is rendered in the time zone asked for, across a DST transition", () => {
  const zone = { timeZone: "America/New_York", dateStyle: "short", timeStyle: "short" } as const;
  const before = formatInstant("en-US", "2026-03-08T06:30:00Z", zone);
  const after = formatInstant("en-US", "2026-03-08T07:30:00Z", zone);

  // One hour of wall clock between them: 01:30 EST becomes 03:30 EDT.
  assert.match(before, /1:30\s*AM/, `expected the pre-transition wall clock, got ${before}`);
  assert.match(after, /3:30\s*AM/, `expected the post-transition wall clock, got ${after}`);
});

test("the same instant reads differently in each zone and identically in UTC", () => {
  const instant = "2026-09-21T08:13:00Z";
  assert.equal(formatInstant("pt-BR", instant, { timeZone: "UTC", timeStyle: "short" }), "08:13");
  assert.equal(formatInstant("pt-BR", instant, { timeZone: "America/Sao_Paulo", timeStyle: "short" }), "05:13");
});

test("an instant that is not one, or a zone that does not exist, is refused", () => {
  assert.throws(() => formatInstant("en-US", "not an instant"), RangeError);
  assert.throws(() => formatInstant("en-US", "2026-13-45T00:00:00Z"), RangeError);
  assert.throws(() => formatInstant("en-US", "2026-09-21T08:13:00Z", { timeZone: "Mars/Phobos" }), RangeError);
});

test("instantOf never hands out the Date it was given", () => {
  const original = new Date("2026-09-21T08:13:00Z");
  const parsed = instantOf(original);

  assert.notEqual(parsed, original);
  assert.equal(parsed.getTime(), original.getTime());
});

test("relative time counts or phrases, as the language does", () => {
  assert.equal(formatRelativeTime("pt-BR", -1, "day"), "ontem");
  assert.equal(formatRelativeTime("en-US", -1, "day"), "yesterday");
  assert.equal(formatRelativeTime("pt-BR", -2, "day", { numeric: "always" }), "há 2 dias");
  assert.equal(formatRelativeTime("en-US", -1, "day", { numeric: "always" }), "1 day ago");
  assert.equal(formatRelativeTime("en-US", 3, "hour", { numeric: "always" }), "in 3 hours");
});

test("a list is joined the way the locale joins it", () => {
  assert.equal(formatList("en-US", ["agree", "disagree", "undecided"]), "agree, disagree, and undecided");
  assert.equal(formatList("pt-BR", ["agree", "disagree", "undecided"]), "agree, disagree e undecided");
  assert.equal(formatList("en-US", ["a", "b"], "disjunction"), "a or b");
  assert.equal(formatList("en-US", ["only"]), "only");
  assert.equal(formatList("en-US", []), "");
});

test("the plural category is the locale's, not a comparison to one", () => {
  // Portuguese groups zero with one; English does not. An implementation
  // comparing the count to one would select `other` for the Portuguese zero.
  assert.equal(pluralCategory("pt-BR", 0), "one");
  assert.equal(pluralCategory("pt-BR", 1), "one");
  assert.equal(pluralCategory("pt-BR", 2), "other");
  assert.equal(pluralCategory("en-US", 0), "other");
  assert.equal(pluralCategory("en-US", 1), "one");
  assert.equal(pluralCategory("en-US", 2), "other");
});
