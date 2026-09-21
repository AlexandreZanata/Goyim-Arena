/**
 * The locale of one page (P18-T09).
 *
 * The document arrives already rendered in a locale the server resolved, so the
 * browser normally renders in that one until it leaves the page. This module is
 * what a page uses when it can do better than a full navigation: it owns the
 * current translator, and every listener that rendered text re-renders with the
 * new one, in place, without reloading the document.
 *
 * It is created by the page — `createLocalization(locale)` — and never reached
 * through a module-level singleton, which is the difference between a page that
 * can be in two languages in one test process and a process-wide language that
 * leaks between pages. What it holds is the *interface* language: the language
 * of the content an Arena was written in (`content_language`) is a property of
 * the Arena and does not move when the reader changes their interface locale.
 */
import { isSupportedLocale } from "./locale.js";
import { createTranslator } from "./translator.js";
import type { Locale } from "./locale.js";
import type { Translator, TranslatorOptions } from "./translator.js";

/** Told when the locale changed, with the translator to render from. */
export type LocaleListener = (translator: Translator, previous: Locale) => void;

/** The localization of one page. */
export interface Localization {
  /** The locale everything rendered text is currently in. */
  readonly locale: Locale;
  /** The translator of that locale. */
  readonly translator: Translator;
  /** switchTo changes the locale and notifies the listeners; it is a no-op for the current one. */
  switchTo(locale: Locale): Translator;
  /** subscribe registers a listener and returns the function that removes it. */
  subscribe(listener: LocaleListener): () => void;
}

/**
 * createLocalization builds the localization of one page.
 *
 * Switching language is a change of the translator, not of the document: every
 * listener receives the new translator and the locale it replaced, so a page can
 * re-render what it rendered before. The locale must be one the product ships —
 * a value that is not is refused, because reflecting it would mean rendering in
 * a language the product does not have.
 */
export function createLocalization(initial: Locale, options: TranslatorOptions = {}): Localization {
  if (!isSupportedLocale(initial)) {
    throw new TypeError(`unsupported interface locale: ${String(initial)}`);
  }

  let current = createTranslator(initial, options);
  const listeners = new Set<LocaleListener>();

  return {
    get locale(): Locale {
      return current.locale;
    },

    get translator(): Translator {
      return current;
    },

    switchTo(locale: Locale): Translator {
      if (!isSupportedLocale(locale)) {
        throw new TypeError(`unsupported interface locale: ${String(locale)}`);
      }
      if (locale === current.locale) {
        return current;
      }
      const previous = current.locale;
      current = createTranslator(locale, options);
      // A listener that subscribes or unsubscribes while being notified must not
      // disturb this pass: the iteration runs over a copy.
      for (const listener of [...listeners]) {
        listener(current, previous);
      }
      return current;
    },

    subscribe(listener: LocaleListener): () => void {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}
