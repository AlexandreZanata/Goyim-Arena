/**
 * Locale identity of the interface (P18-T09, I18N_STANDARD.md sections 1 and 4).
 *
 * The server resolves the locale of a request — explicit route or action value,
 * authenticated preference, validated cookie, `Accept-Language` negotiated
 * against the allowlist, then the configured default — and renders the document
 * with it. The browser therefore never negotiates anything: it *reads* the
 * resolved locale and refuses to trust anything else it finds, which is the rule
 * that a value the product does not support is never reflected back. A query
 * string, a cookie or a `lang` attribute is untrusted input like any other.
 *
 * Canonicalisation is delegated to `Intl.getCanonicalLocales`, so "PT-br" and
 * "pt-br" both name the supported `pt-BR` while a malformed tag throws inside
 * ICU and is treated as absent here.
 */

/** The interface locales the product ships catalogs for. */
export type Locale = "pt-BR" | "en-US";

/**
 * The locale of the product when nothing supported was offered. It is a product
 * decision, never a guess derived from the visitor's address (I18N_STANDARD.md
 * section 1), and it is the same locale the server falls back to.
 */
export const defaultLocale: Locale = "pt-BR";

/**
 * Every supported locale, in the order the interface offers them. The generated
 * catalog is the source of truth for which locales exist: `web/tests/i18n`
 * fails if `messages` grows a locale this list does not know, or loses one it
 * still names.
 */
export const supportedLocales: readonly Locale[] = ["pt-BR", "en-US"];

/**
 * canonicalTag returns the BCP 47 canonical form of a candidate tag, or null
 * when the candidate is not a well-formed language tag at all.
 *
 * `Intl.getCanonicalLocales` rejects anything malformed instead of guessing, and
 * that rejection — not a hand-written grammar — is what keeps a value from
 * entering the product merely because it looks like a locale.
 */
export function canonicalTag(candidate: unknown): string | null {
  if (typeof candidate !== "string") {
    return null;
  }
  const trimmed = candidate.trim();
  if (trimmed === "") {
    return null;
  }
  try {
    const [canonical] = Intl.getCanonicalLocales(trimmed);
    return canonical ?? null;
  } catch {
    return null;
  }
}

/**
 * canonicalLocale names the supported locale a candidate means, or null when it
 * means none. A non-canonical spelling of a supported locale is accepted and
 * normalised — "pt-br" is the same language as "pt-BR" — while a value that is
 * merely close to one ("pt", "pt-PT", "en") is not: the product ships the
 * catalogs it ships, and serving `pt-PT` from the `pt-BR` catalog would be a
 * translation decision nobody made.
 */
export function canonicalLocale(candidate: unknown): Locale | null {
  const canonical = canonicalTag(candidate);
  if (canonical === null) {
    return null;
  }
  for (const locale of supportedLocales) {
    if (locale === canonical) {
      return locale;
    }
  }
  return null;
}

/**
 * isSupportedLocale answers whether a value *is* one of the shipped locales,
 * already canonical. Unlike `canonicalLocale` it normalises nothing, so it is
 * safe as a type guard: a string it accepts is exactly a `Locale`.
 */
export function isSupportedLocale(candidate: unknown): candidate is Locale {
  return typeof candidate === "string" && supportedLocales.some((locale) => locale === candidate);
}

/**
 * resolveLocale returns the first candidate the product supports, and the
 * default when none is. It is the browser-side counterpart of the server's
 * precedence list, and it only ever returns a locale from the allowlist: an
 * unsupported candidate is skipped, never answered with.
 */
export function resolveLocale(candidates: Iterable<unknown>): Locale {
  for (const candidate of candidates) {
    const locale = canonicalLocale(candidate);
    if (locale !== null) {
      return locale;
    }
  }
  return defaultLocale;
}
