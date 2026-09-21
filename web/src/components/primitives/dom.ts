/**
 * Minimal DOM helpers of the primitives (P18-T04).
 *
 * Light DOM is the project default (AGENTS.md section 2), so the primitives
 * adopt what the server already rendered and only create a node when it is
 * missing. Text is always written through `textContent`: dynamic data never
 * goes through `innerHTML` (docs/FRONTEND.md section 9).
 */

/** Writes the given attributes, leaving the others untouched. */
export function setAttributes(element: Element, attributes: Readonly<Record<string, string>>): void {
  for (const name of Object.keys(attributes)) {
    const value = attributes[name];
    if (value !== undefined) {
      element.setAttribute(name, value);
    }
  }
}

/** Removes the named attributes, so stale state never survives a render. */
export function removeAttributes(element: Element, names: readonly string[]): void {
  for (const name of names) {
    element.removeAttribute(name);
  }
}

/**
 * replaceAttributes clears the attributes this primitive manages and then
 * applies the wanted ones: a render never leaves a half-updated element.
 */
export function replaceAttributes(
  element: Element,
  managed: readonly string[],
  attributes: Readonly<Record<string, string>>,
): void {
  removeAttributes(element, managed);
  setAttributes(element, attributes);
}

/** Writes text content; the only way dynamic data reaches the DOM here. */
export function setText(element: Element, content: string): void {
  element.textContent = content;
}

/** Shows or hides a node without removing it from the document. */
export function toggleHidden(element: Element, hidden: boolean): void {
  if (hidden) {
    element.setAttribute("hidden", "");
    return;
  }
  element.removeAttribute("hidden");
}

/**
 * ensureChild adopts the first descendant matching `selector`, or creates
 * `tagName` at the requested position. The cast is the single unchecked step
 * at the DOM boundary; callers only use it with selectors they wrote.
 */
export function ensureChild<T extends Element>(
  parent: Element,
  selector: string,
  tagName: string,
  position: "start" | "end" = "end",
): T {
  const existing = parent.querySelector(selector);
  if (existing !== null) {
    return existing as unknown as T;
  }
  const created = parent.ownerDocument.createElement(tagName);
  if (position === "start") {
    parent.prepend(created);
  } else {
    parent.append(created);
  }
  return created as unknown as T;
}

/**
 * findControl locates the control of this field: an explicit
 * `[data-ga-control]` wins, otherwise the first form control that belongs to
 * this field and not to a nested one.
 */
export function findControl(root: Element): HTMLElement | null {
  const explicit = root.querySelector("[data-ga-control]");
  if (explicit instanceof HTMLElement) {
    return explicit;
  }
  for (const candidate of root.querySelectorAll("input, select, textarea")) {
    if (candidate instanceof HTMLElement && candidate.closest("ga-field") === root) {
      return candidate;
    }
  }
  return null;
}

/**
 * Focuses an element resolved from the document, if it exists. Elements that
 * cannot hold focus on their own (a heading, a paragraph) receive `tabindex`
 * `-1` first, so the summary link lands exactly where the user must look.
 */
export function focusById(document: Document, identifier: string): boolean {
  const target = document.getElementById(identifier);
  if (target === null) {
    return false;
  }
  if (target.tabIndex < 0) {
    target.setAttribute("tabindex", "-1");
  }
  target.focus();
  return true;
}
