/**
 * Entry module of the Arena participation page (P18-T06).
 *
 * It is loaded as a native ES module by the page and it is an enhancer and
 * nothing else. Every transition already works without it — the server renders
 * the forms, validates every value against the same closed vocabulary, refuses
 * what it must and answers a document or a redirect — so removing this file
 * changes nothing about what the product accepts. What it adds is what a
 * document cannot do on its own:
 *
 *   - the local position choice of a visitor, kept in this browser and never
 *     sent anywhere, used to start the confirmation form from what the person
 *     already picked;
 *   - the refusal of a submission already in flight, so an impatient double
 *     click does not publish twice (the attempt key of the publication form is
 *     the server-side guarantee; this is the courtesy);
 *   - the refusal to tick past the attribution limit the page declares.
 *
 * Registration of the primitives and the submission guard come from the account
 * journey's module: the two surfaces share the same document conventions
 * (`ga-busy`, `ga-error-summary`, `ga-toast`), and the browser policy of this
 * binary refuses inline code, so the module is the only script the page loads.
 */
import { installSubmissionGuard } from "./auth.js";
import {
  ARENA_ATTRIBUTE,
  ATTRIBUTION_GROUP_SELECTOR,
  CHOICE_ATTRIBUTE,
  CHOICE_GROUP_SELECTOR,
  attributionSelection,
  choiceStorageKey,
  chooseLocalChoice,
  readLocalChoice,
} from "./participation.js";
import type { Position } from "./participation.js";

/** The storage the page may use, or null when the browser refuses it. */
function storageOf(): Storage | null {
  try {
    const storage = globalThis.localStorage;
    // A browser configured to refuse storage throws on access, and a page
    // that renders either way must not break because of it.
    const probe = "__ga_probe__";
    storage.setItem(probe, "1");
    storage.removeItem(probe);
    return storage;
  } catch {
    return null;
  }
}

/** The Arena the page belongs to, or the empty string when it declares none. */
function arenaOf(document: Document): string {
  const group = document.querySelector(CHOICE_GROUP_SELECTOR);
  if (group === null) {
    return "";
  }
  return (group.getAttribute(ARENA_ATTRIBUTE) ?? "").trim();
}

/** Marks one choice button as the chosen one, and every other as unchosen. */
function applyChoice(group: Element, position: Position | null): void {
  for (const button of group.querySelectorAll(`[${CHOICE_ATTRIBUTE}]`)) {
    const value = (button.getAttribute(CHOICE_ATTRIBUTE) ?? "").trim();
    if (position !== null && value === position) {
      button.setAttribute("aria-pressed", "true");
    } else {
      button.setAttribute("aria-pressed", "false");
    }
  }
}

/**
 * preselectPosition checks the stored choice in the position form, and only when
 * the server did not check one already: the stored projection of an owner is the
 * server's answer and outranks anything this browser remembers.
 */
function preselectPosition(document: Document, position: Position | null): void {
  if (position === null) {
    return;
  }
  const form = document.querySelector('form input[name="position"]');
  if (form === null) {
    return;
  }
  const group = form.closest("fieldset");
  if (group === null) {
    return;
  }
  for (const radio of group.querySelectorAll('input[name="position"]')) {
    if (radio instanceof HTMLInputElement && radio.checked) {
      return;
    }
  }
  const target = group.querySelector(`input[name="position"][value="${position}"]`);
  if (target instanceof HTMLInputElement) {
    target.checked = true;
  }
}

/** Installs the local choice of a visitor and the preselection of the form. */
function installLocalChoice(document: Document, arenaID: string): void {
  const storage = storageOf();
  const stored = storage === null || arenaID === "" ? null : readLocalChoice(storage.getItem(choiceStorageKey(arenaID)));

  const group = document.querySelector(CHOICE_GROUP_SELECTOR);
  if (group !== null) {
    applyChoice(group, stored);
    group.addEventListener("click", (event: Event): void => {
      const target = event.target;
      if (!(target instanceof Element)) {
        return;
      }
      const button = target.closest(`[${CHOICE_ATTRIBUTE}]`);
      if (button === null || !group.contains(button)) {
        return;
      }
      const decision = chooseLocalChoice(button.getAttribute(CHOICE_ATTRIBUTE) ?? "");
      if (!decision.accepted || decision.position === null) {
        return;
      }
      if (storage !== null && arenaID !== "") {
        storage.setItem(choiceStorageKey(arenaID), decision.position);
      }
      applyChoice(group, decision.position);
    });
  }

  preselectPosition(document, stored);

  // The confirmation is the moment the local choice stops being local: the
  // server recorded it, and a stale copy in this browser must not survive it.
  for (const form of document.querySelectorAll('form[action$="/position"]')) {
    form.addEventListener("submit", (): void => {
      if (storage !== null && arenaID !== "") {
        storage.removeItem(choiceStorageKey(arenaID));
      }
    });
  }
}

/**
 * installAttributionLimit refuses the tick that would pass the limit the page
 * declares. It never unchecks a selection the person made within the limit, and
 * it marks the group as invalid so the refusal is visible instead of silent.
 */
function installAttributionLimit(document: Document, limit: number): void {
  const group = document.querySelector(ATTRIBUTION_GROUP_SELECTOR);
  if (group === null || limit < 1) {
    return;
  }
  group.addEventListener("change", (event: Event): void => {
    const target = event.target;
    if (!(target instanceof HTMLInputElement) || target.type !== "checkbox") {
      return;
    }
    // The selection as it was before this toggle, in the order the page
    // listed it: the decision is about adding one argument to it.
    const previous: string[] = [];
    for (const checkbox of group.querySelectorAll('input[type="checkbox"]')) {
      if (checkbox instanceof HTMLInputElement && checkbox.checked && checkbox !== target) {
        previous.push(checkbox.value);
      }
    }
    const decision = attributionSelection(previous, target.value, target.checked, limit);
    if (!decision.allowed) {
      target.checked = false;
      group.setAttribute("aria-invalid", "true");
      return;
    }
    group.removeAttribute("aria-invalid");
  });
}

/** The limit the page declares for one attribution, or zero when it declares none. */
function attributionLimit(document: Document): number {
  const group = document.querySelector(ATTRIBUTION_GROUP_SELECTOR);
  if (group === null) {
    return 0;
  }
  const declared = Number.parseInt(group.getAttribute("data-ga-attribution-limit") ?? "", 10);
  return Number.isFinite(declared) ? declared : 0;
}

/** Installs the whole enhancement on one document. */
export function installArenaPage(document: Document = globalThis.document): void {
  installSubmissionGuard(document);
  installLocalChoice(document, arenaOf(document));
  installAttributionLimit(document, attributionLimit(document));
}

installArenaPage();
