/**
 * The pseudo-locale gate in a real browser (P18-T10).
 *
 * The catalog of the pseudo-locale is derived from the default one: every
 * message is bracketed, its letters accented and its text measurably longer.
 * Driving the pages with `Accept-Language: qps-Ploc` therefore asks a question
 * no unit test can answer — does the delivered layout survive the text a
 * translation will bring? — and it answers it on the journeys a person takes,
 * not on a fixture: the account page and the participation page, rendered by
 * the delivered binary, with the delivered stylesheets.
 *
 * Four properties are checked on each document, and each one fails for a
 * different reason:
 *
 *   - the locale reached the document: `lang` is the negotiated tag and the
 *     body carries the pseudo markers. A page that negotiates one locale and
 *     renders another is invisible to every other assertion here.
 *   - the direction is declared: `dir` is present and matches the language.
 *   - nothing overflows: no element reaches past the viewport and the document
 *     does not scroll sideways. This is the property the elongated text
 *     exists to break, and a served interface that only fits Portuguese is an
 *     interface that breaks in every translation.
 *   - the accessible structure still holds: every label, description and
 *     labelled-by reference points at an element that exists, and the keyboard
 *     reaches the controls in a form a person can complete.
 *
 * The control run is the other half: the same pages in the default locale must
 * carry no pseudo text at all, so the markers prove the pseudo catalog was
 * served rather than a copy that happens to contain a bracket.
 */
import { expect, test } from "@playwright/test";
import { participationEnvironment } from "../support/environment.js";
import { signIn } from "../support/account.js";

/** The pseudo-locale the harness built the server with, and its markers. */
const PSEUDO_LOCALE = process.env.ARENA_E2E_PSEUDO_LOCALE ?? "qps-Ploc";
const MARKER_OPEN = "⟦";
const MARKER_CLOSE = "⟧";

/** Sub-pixel rounding is not an overflow; a clipped glyph is. */
const OVERFLOW_TOLERANCE = 1;

/**
 * openPseudoPage opens one page in the pseudo-locale. The locale is a browser
 * context option, so it travels as the `Accept-Language` header the server
 * negotiates — the same path a real visitor takes, never a query parameter
 * this product does not have.
 */
async function openPseudoPage(browser, path, viewport) {
  const options = { locale: PSEUDO_LOCALE };
  if (viewport !== undefined) {
    options.viewport = viewport;
  }
  const context = await browser.newContext(options);
  const page = await context.newPage();
  await page.goto(path);
  return { context, page };
}

/**
 * NARROW is the viewport of a phone, where a layout that gained half of its
 * text the longest it will ever be is closest to its budget. The widths are
 * the ones the stylesheets declare breakpoints for.
 */
const NARROW = { width: 360, height: 640 };

/**
 * inspectDocument reads everything the assertions need from the rendered
 * document in one round trip: the declared language and direction, the
 * elements that reach past the viewport, the references the accessible
 * structure depends on, and the text a person reads.
 */
async function inspectDocument(page, tolerance) {
  return page.evaluate((maximum) => {
    const root = document.documentElement;
    const viewport = root.clientWidth;

    /** A short human name of one element, for a failure message. */
    const describe = (element) => {
      const first = String(element.className || "").split(" ").filter(Boolean)[0];
      return first === undefined
        ? element.tagName.toLowerCase()
        : `${element.tagName.toLowerCase()}.${first}`;
    };

    const offenders = [];
    for (const element of document.querySelectorAll("body *")) {
      const rect = element.getBoundingClientRect();
      if (rect.width === 0 && rect.height === 0) {
        continue;
      }
      const style = window.getComputedStyle(element);
      if (style.visibility === "hidden" || style.clipPath !== "none") {
        continue;
      }
      if (rect.right > viewport + maximum || rect.left < -maximum) {
        offenders.push(`${describe(element)}[${Math.round(rect.left)}..${Math.round(rect.right)}]`);
      }
    }

    // Every reference the accessible structure makes must resolve to an
    // element that is in the document.
    const missingReferences = [];
    const referenceable = ["aria-describedby", "aria-labelledby", "aria-controls", "for"];
    for (const element of document.querySelectorAll("body *")) {
      for (const attribute of referenceable) {
        const value = element.getAttribute(attribute);
        if (value === null || value.trim() === "") {
          continue;
        }
        for (const id of value.split(/\s+/)) {
          if (document.getElementById(id) === null) {
            missingReferences.push(`${describe(element)}@${attribute}=${id}`);
          }
        }
      }
    }

    return {
      lang: root.getAttribute("lang"),
      dir: root.getAttribute("dir"),
      viewport,
      scrollWidth: root.scrollWidth,
      offenders: offenders.slice(0, 8),
      missingReferences: missingReferences.slice(0, 8),
      text: document.body.innerText,
      placeholderLeftovers: (document.body.innerText.match(/\{[a-z_]+\}/g) ?? []).slice(0, 5),
      catalogKeys: (
        document.body.innerText.match(/\b(?:auth|arenas|errors|email|transparency)\.[a-z_]+(?:\.[a-z_]+)+/g) ?? []
      ).slice(0, 5),
    };
  }, tolerance);
}

/** assertDocumentIsSound applies the four properties of the gate. */
function assertDocumentIsSound(report, { locale, pseudo, label }) {
  expect(report.lang, `${label}: the document declares its locale`).toBe(locale);
  expect(report.dir, `${label}: the document declares its direction`).toBe("ltr");

  expect(report.placeholderLeftovers, `${label}: no placeholder was left unreplaced`).toEqual([]);
  expect(report.catalogKeys, `${label}: no catalog key leaked into the page`).toEqual([]);
  expect(report.missingReferences, `${label}: every aria and label reference resolves`).toEqual([]);
  expect(
    report.scrollWidth,
    `${label}: the document does not scroll sideways (viewport ${report.viewport}px)`,
  ).toBeLessThanOrEqual(report.viewport + OVERFLOW_TOLERANCE);
  expect(report.offenders, `${label}: no element reaches past the viewport`).toEqual([]);

  if (pseudo) {
    expect(report.text.includes(MARKER_OPEN) && report.text.includes(MARKER_CLOSE), `${label}: the pseudo catalog was served`).toBe(true);
  } else {
    expect(report.text.includes(MARKER_OPEN), `${label}: the default locale serves no pseudo text`).toBe(false);
  }
}

test("the pseudo-locale reaches the documents and their layout survives it", async ({ browser }) => {
  // The two documents a visitor meets before signing in. The journeys stay
  // inside the surfaces the composed server mounts: an address it does not
  // serve would answer a page without any locale at all, and the gate would
  // report the router instead of the layout.
  for (const path of ["/login", "/register"]) {
    const opened = await openPseudoPage(browser, path);
    try {
      const report = await inspectDocument(opened.page, OVERFLOW_TOLERANCE);
      assertDocumentIsSound(report, { locale: PSEUDO_LOCALE, pseudo: true, label: path });
      expect(report.text.length, `${path}: the page rendered content`).toBeGreaterThan(100);
    } finally {
      await opened.context.close();
    }
  }

  // The same document on a phone: the widened text is what the narrow
  // viewport has the least room for, so the two together are the question
  // the gate exists to ask.
  const narrow = await openPseudoPage(browser, "/login", NARROW);
  try {
    const report = await inspectDocument(narrow.page, OVERFLOW_TOLERANCE);
    assertDocumentIsSound(report, { locale: PSEUDO_LOCALE, pseudo: true, label: "/login at 360px" });
  } finally {
    await narrow.context.close();
  }
});

test("the participation journey stays completable in the pseudo-locale", async ({ browser }) => {
  const environment = participationEnvironment();

  const context = await browser.newContext({ locale: PSEUDO_LOCALE });
  try {
    const page = await context.newPage();
    // Signing in is the first honest proof that the pseudo-locale did not
    // change what the form accepts: the fields keep their names and the
    // submission keeps its token.
    await signIn(page, { email: environment.participantEmail, password: environment.participantPassword });
    await page.goto(`/arenas/${environment.arenaSlug}`);

    assertDocumentIsSound(await inspectDocument(page, OVERFLOW_TOLERANCE), {
      locale: PSEUDO_LOCALE,
      pseudo: true,
      label: "participation",
    });

    // The keyboard reaches the controls of the form, and each control it
    // reaches is one a person can identify: a focusable element without a
    // name is a control only a mouse can use.
    const form = page.locator('form[action$="/position"]').first();
    await expect(form).toBeVisible();

    const reachable = await form.evaluate((element) => {
      const controls = [...element.querySelectorAll("input, button, select, textarea")].filter(
        (control) => control.type !== "hidden" && !control.disabled,
      );
      return controls.map((control) => {
        const labelled = control.labels !== undefined && control.labels !== null && control.labels.length > 0;
        const attributes = ["aria-label", "aria-labelledby", "title"].some(
          (attribute) => (control.getAttribute(attribute) ?? "").trim() !== "",
        );
        // A button is named by the text it shows; a field is named by its
        // label. Both are names a person can read, which is what the check
        // is about.
        const named = labelled || attributes || control.textContent.trim() !== "";
        return { name: control.getAttribute("name") ?? control.tagName.toLowerCase(), named };
      });
    });
    expect(reachable.length, "participation: the position form has controls").toBeGreaterThan(0);
    for (const control of reachable) {
      expect(control.named, `participation: the control ${control.name} has an accessible name`).toBe(true);
    }

    await page.locator('form[action$="/position"] input[name="position"]').first().focus();
    await expect(page.locator('form[action$="/position"] input[name="position"]').first()).toBeFocused();
  } finally {
    await context.close();
  }
});

test("the same pages carry no pseudo text in the default locale", async ({ browser }) => {
  // The control: without it, an assertion on a bracket would prove only that
  // the assertion can see a bracket.
  const context = await browser.newContext({ locale: "pt-BR" });
  try {
    const page = await context.newPage();
    await page.goto("/login");
    const report = await inspectDocument(page, OVERFLOW_TOLERANCE);
    assertDocumentIsSound(report, { locale: "pt-BR", pseudo: false, label: "account control" });
  } finally {
    await context.close();
  }
});
