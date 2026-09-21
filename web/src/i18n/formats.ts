/**
 * Formatting of the interface through the native `Intl` APIs (P18-T09,
 * I18N_STANDARD.md sections 5 and 6).
 *
 * Three rules shape this module, and each one is a rule the product already
 * holds on the server side:
 *
 *   - the locale is always explicit. Nothing here formats through the implicit-
 *     locale `toLocale*String` family, which would use whatever locale the
 *     reader's browser happens to have configured instead of the locale the page
 *     was rendered in;
 *   - money is never a float. The API sends integer minor units plus an ISO
 *     currency, so the amount is assembled from integers and the *locale* keeps
 *     its own decimal separator, group separator, symbol placement and digit
 *     count — read from `Intl`, never hardcoded;
 *   - nothing is cached in module scope. A formatter cache would be mutable
 *     state shared by every page and every locale of one document, and the
 *     engines already memoise their own `Intl` data.
 */
import type { Locale } from "./locale.js";

/** Relative units the interface may need to phrase. */
export type RelativeUnit = "year" | "quarter" | "month" | "week" | "day" | "hour" | "minute" | "second";

/** How a list joins its items: "a, b and c" or "a, b or c". */
export type ListStyle = "conjunction" | "disjunction";

/** The lengths `Intl.DateTimeFormat` names. */
export type DateLength = "full" | "long" | "medium" | "short";

/** Options of one instant rendering; the time zone is an explicit preference. */
export interface InstantOptions {
  readonly timeZone?: string;
  readonly dateStyle?: DateLength;
  readonly timeStyle?: DateLength;
}

/** formatNumber renders a number with the locale's grouping and separators. */
export function formatNumber(locale: Locale, value: number | bigint, options: Intl.NumberFormatOptions = {}): string {
  return new Intl.NumberFormat(locale, { useGrouping: true, ...options }).format(value);
}

/**
 * currencyFractionDigits reports how many minor units the currency uses, as the
 * locale itself declares them (two for BRL and USD, none for JPY, three for
 * BHD). Reading the number from `Intl` is what keeps a new currency from
 * silently formatting with the wrong scale.
 */
export function currencyFractionDigits(locale: Locale, currency: string): number {
  const resolved = new Intl.NumberFormat(locale, { style: "currency", currency }).resolvedOptions();
  // The shape of the resolved options makes the digit count optional because not
  // every style has one; a currency style always resolves it. The fallback keeps
  // the reading exhaustive without inventing a scale for a currency ICU knows.
  return resolved.maximumFractionDigits ?? 2;
}

/**
 * formatCurrencyMinor formats an integer amount in minor units, without ever
 * letting it pass through binary floating point.
 *
 * The assembly is deliberate. `Intl.NumberFormat` takes a `Number` or a
 * `BigInt`, and for money the `BigInt` path is the only exact one: the amount is
 * split into whole and fractional minor units with integer arithmetic, and the
 * locale's own rendering is used as the *template* for everything around them —
 * currency symbol, its position, the decimal separator, the minus sign — so a
 * convention is never reimplemented here. A fractional or unsafe number is
 * refused instead of rounded, because an amount that cannot be represented
 * exactly is a defect to fix at the boundary, not to format quietly.
 */
export function formatCurrencyMinor(locale: Locale, minorUnits: number | bigint, currency: string): string {
  const digits = currencyFractionDigits(locale, currency);
  const scaled = asMinorUnits(minorUnits);
  const negative = scaled < 0n;
  const absolute = negative ? -scaled : scaled;
  const scale = 10n ** BigInt(digits);
  const whole = absolute / scale;
  const fraction = absolute % scale;

  const template = new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
  const grouped = new Intl.NumberFormat(locale, { useGrouping: true, maximumFractionDigits: 0 }).formatToParts(whole);
  const integerText = grouped
    .filter((part) => part.type === "integer" || part.type === "group")
    .map((part) => part.value)
    .join("");
  const fractionText = fraction.toString().padStart(digits, "0");

  // The parts of a sample render, with only the digits replaced: the sign, the
  // symbol and the separators keep the position and the value ICU gives them.
  return template
    .formatToParts(negative ? -1 : 1)
    .map((part) => {
      if (part.type === "integer") {
        return integerText;
      }
      if (part.type === "fraction") {
        return fractionText;
      }
      return part.value;
    })
    .join("");
}

/** asMinorUnits refuses anything that is not an exact integer count. */
function asMinorUnits(value: number | bigint): bigint {
  if (typeof value === "bigint") {
    return value;
  }
  if (!Number.isSafeInteger(value)) {
    throw new TypeError(`minor units must be a safe integer, received ${String(value)}`);
  }
  return BigInt(value);
}

/**
 * formatInstant renders one instant, given as an RFC 3339 string (what the API
 * sends) or a `Date`, in the locale and the time zone asked for.
 *
 * The time zone is a preference independent of the locale (I18N_STANDARD.md
 * section 1) and it is never inferred: without one the reader's own zone is
 * used, and a feature that needs a fixed zone asks for it by name. An invalid
 * zone or an unparseable instant is refused by ICU or by `instantOf`, never
 * rendered as `Invalid Date`.
 */
export function formatInstant(locale: Locale, instant: string | Date, options: InstantOptions = {}): string {
  return new Intl.DateTimeFormat(locale, { ...options }).format(instantOf(instant));
}

/** instantOf parses an instant and refuses anything that is not one. */
export function instantOf(instant: string | Date): Date {
  const parsed = instant instanceof Date ? new Date(instant.getTime()) : new Date(instant);
  if (Number.isNaN(parsed.getTime())) {
    throw new RangeError(`not an RFC 3339 instant: ${String(instant)}`);
  }
  return parsed;
}

/**
 * formatRelativeTime phrases a distance in time. With `numeric: "auto"` a
 * distance of one unit is phrased the way the language phrases it ("ontem",
 * "yesterday") instead of counting it ("há 1 dia", "1 day ago").
 */
export function formatRelativeTime(
  locale: Locale,
  value: number,
  unit: RelativeUnit,
  options: { readonly numeric?: "always" | "auto" } = {},
): string {
  return new Intl.RelativeTimeFormat(locale, { numeric: options.numeric ?? "auto" }).format(value, unit);
}

/** formatList joins items the way the locale joins them. */
export function formatList(locale: Locale, items: readonly string[], style: ListStyle = "conjunction"): string {
  if (items.length === 0) {
    return "";
  }
  return new Intl.ListFormat(locale, { style: "long", type: style }).format(items);
}

/**
 * pluralCategory is the CLDR plural category of a count in one locale. It is the
 * only correct way to choose a plural form: the categories are per language — in
 * `pt-BR` both zero and one select `one`, in `en-US` zero selects `other`, and
 * languages the product does not ship yet have categories such as `zero`, `two`
 * and `few`. Comparing a count to one would be right in English and wrong
 * everywhere else.
 */
export function pluralCategory(locale: Locale, count: number): Intl.LDMLPluralRule {
  return new Intl.PluralRules(locale).select(count);
}
