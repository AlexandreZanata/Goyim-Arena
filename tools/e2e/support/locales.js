/**
 * The language dimension of the browser journeys (P20-T09).
 *
 * The journeys of P18-T07 were written in the language the harness happened to
 * negotiate, which means "the journeys pass" was a statement about one locale.
 * The release audit requires the opposite: every journey is driven in every
 * interface locale the product ships, and each one holds the pages it reached
 * to the language it asked for.
 *
 * Two assertions, because the product has two kinds of document and they are
 * localized for different reasons (I18N_STANDARD.md §1 and §7):
 *
 *   - the interface pages (account, participation forms, refusals) are rendered
 *     in the locale the request negotiated, so the document must declare
 *     exactly the locale the browser context asked for — read from the page
 *     itself (`navigator.language`), never from a constant the journey carries,
 *     so a journey that forgot to ask for a locale cannot pass by asserting the
 *     default against itself;
 *   - an Arena document (and the surface that presents it) is rendered in its
 *     `content_language`, which is a property of the Arena and not of the
 *     reader. The journeys assert it against the language the harness seeded,
 *     which is what makes "reading the same Arena in another interface locale
 *     never changes the Arena" a measured fact instead of a comment.
 *
 * `JOURNEY_LOCALES` is the list the specs iterate over. It is read from this
 * file by `tools/i18nrelease`, which refuses a register that claims coverage of
 * a locale the journeys do not actually ask for.
 */
import { expect } from "@playwright/test";
import { contentLanguage } from "./environment.js";

/** The interface locales every journey of the suite is driven in, in order. */
export const JOURNEY_LOCALES = ["pt-BR", "en-US"];

/**
 * coveredLocales is every locale a context of this run may ask for: the locales
 * of the suite, plus the pseudo-locale when the harness built the server with
 * it. The pseudo-locale is deliberately outside `JOURNEY_LOCALES` — it is not a
 * locale the product ships — and it is included here because the layout gate
 * drives real pages with it.
 */
function coveredLocales() {
  const pseudo = (process.env.ARENA_E2E_PSEUDO_LOCALE ?? "").trim();
  return pseudo === "" ? JOURNEY_LOCALES : [...JOURNEY_LOCALES, pseudo];
}

/** documentLanguage reads the language and direction a document declares. */
async function documentLanguage(page) {
  return page.evaluate(() => ({
    declared: document.documentElement.lang,
    direction: document.documentElement.dir,
    asked: navigator.language,
  }));
}

/**
 * expectInterfaceLanguage asserts that the document on screen is the one the
 * context asked for: the language it declares is the language the browser
 * reported, and that language is one of the locales the journeys cover.
 *
 * The second half is the guard against vacuity. Without it a context created
 * with no locale would report the runner's default on both sides and the
 * assertion would hold while proving nothing about negotiation.
 */
export async function expectInterfaceLanguage(page, label) {
  const report = await documentLanguage(page);
  expect(coveredLocales(), `${label}: the context asked for a locale the server was built to serve`).toContain(report.asked);
  expect(report.declared, `${label}: the document declares the language the context asked for`).toBe(report.asked);
  // Direction is declared from the same locale as the language (P18-T10); the
  // two supported locales are both left-to-right, and the pseudo-locale gate
  // covers the attribute itself.
  expect(["ltr", "rtl"], `${label}: the document declares a direction`).toContain(report.direction);
}

/**
 * expectContentLanguage asserts that the page states the language of what it
 * renders, whatever interface locale the reader asked for.
 *
 * This is the invariant §1 states and §7 applies: user content is never
 * translated, and the interface locale does not reach it. An Arena has a
 * `content_language` of its own, and the participation surface states it in a
 * sentence of its own — the sentence is translated, the value in it is not.
 * The assertion is therefore about the two halves separately: the paragraph
 * holds the language the harness seeded, in every locale the journey runs in,
 * while the label around it comes from the catalog (which
 * `expectInterfaceLanguage` holds to the locale the context asked for).
 *
 * It is asserted on the delivered surface, not on the public Arena document:
 * the document route is implemented and tested but not composed in the process
 * this suite drives — that gap is I18N-05 in docs/I18N_AUDIT.md, and a journey
 * that asserted a page nobody serves would be asserting a test.
 */
export async function expectContentLanguage(page, label) {
  const language = contentLanguage();
  const stated = await page.locator("#main p", { hasText: language }).count();
  expect(
    stated,
    `${label}: the page states the content language ${language} it was rendered for, in this interface locale too`,
  ).toBeGreaterThan(0);
}
