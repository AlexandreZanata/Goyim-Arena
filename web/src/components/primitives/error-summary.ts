/**
 * `ga-error-summary` — form error summary (P18-T04).
 *
 * The server renders the errors of a failed submission as a list of links
 * inside this element, and the element adopts that list on connect: it reads
 * the links, keeps the document order and completes the markup with its own
 * classes and behavior instead of wiping what the person has to read. Focusing
 * the summary on adoption is the submit flow's focus move — the page only
 * arrives with errors after a submission the server refused.
 *
 * A page may also assign `errors` (already translated messages keyed by field
 * id); the element renders a linked list inside itself, becomes a landmark for
 * assistive technology and moves focus to the offending field when a link is
 * activated, so keyboard users never have to hunt for it. An assignment owns
 * the summary from then on.
 *
 * Without JavaScript the server list stays as static markup: the summary is
 * informative, and the fields keep their own error paragraphs.
 */
import { ensureChild, focusById, setText, toggleHidden } from "./dom.js";
import { errorSummaryItems, focusTargetId, summaryErrors } from "./model.js";
import type { ErrorSummaryItem, FieldError, SummaryLink } from "./model.js";

const LIST_CLASS = "ga-error-summary__list";
const ITEM_CLASS = "ga-error-summary__item";
const LINK_CLASS = "ga-error-summary__link";

export class GaErrorSummaryElement extends HTMLElement {
  private current: readonly ErrorSummaryItem[] = [];

  connectedCallback(): void {
    this.classList.add("ga-error-summary");
    if (!this.hasAttribute("tabindex")) {
      this.setAttribute("tabindex", "-1");
    }
    this.addEventListener("click", this.onActivate);
    this.adopt();
    this.render();
  }

  disconnectedCallback(): void {
    this.removeEventListener("click", this.onActivate);
  }

  /**
   * adopt reads the summary the server rendered, so the upgrade completes it
   * instead of wiping it. It runs once, before the first render: an assignment
   * made before the element connects already owns the summary, and an
   * assignment made later replaces what was adopted.
   */
  private adopt(): void {
    if (this.current.length > 0) {
      return;
    }
    const links: SummaryLink[] = [];
    for (const link of this.querySelectorAll("ul a[href]")) {
      links.push({ href: link.getAttribute("href") ?? "", message: link.textContent ?? "" });
    }
    this.errors = summaryErrors(links);
  }

  /** Errors of the form; assigning an empty list hides the summary. */
  set errors(errors: readonly FieldError[]) {
    const previous = this.current.length;
    this.current = errorSummaryItems(errors);
    this.render();
    if (previous === 0 && this.current.length > 0) {
      this.focusSummary();
    }
  }

  get errors(): readonly ErrorSummaryItem[] {
    return this.current;
  }

  /** Moves focus onto the summary itself, as the submit flow requires. */
  focusSummary(): void {
    this.focus();
  }

  /** Renders the list, adopting a server-rendered one when present. */
  render(): void {
    const list = ensureChild<HTMLUListElement>(this, ":scope > ul", "ul");
    list.classList.add(LIST_CLASS);
    list.replaceChildren();
    for (const item of this.current) {
      const entry = list.ownerDocument.createElement("li");
      entry.classList.add(ITEM_CLASS);
      const link = list.ownerDocument.createElement("a");
      link.classList.add(LINK_CLASS);
      link.setAttribute("href", item.href);
      // Dynamic data reaches the DOM as a text node only (no innerHTML).
      setText(link, item.message);
      entry.append(link);
      list.append(entry);
    }
    const empty = this.current.length === 0;
    if (empty) {
      this.removeAttribute("role");
    } else {
      this.setAttribute("role", "alert");
    }
    toggleHidden(list, empty);
    toggleHidden(this, empty);
  }

  /** Moves focus to the field a summary link points at. */
  private readonly onActivate = (event: Event): void => {
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    const link = target.closest("a[href]");
    if (link === null) {
      return;
    }
    const identifier = focusTargetId(link.getAttribute("href") ?? "");
    if (identifier === null) {
      return;
    }
    // The fragment still updates (the URL stays shareable); only the focus
    // moves, and only when the target actually exists.
    focusById(this.ownerDocument, identifier);
  };
}
