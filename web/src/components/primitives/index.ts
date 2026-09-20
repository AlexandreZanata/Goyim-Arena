/**
 * Registration of the native UI primitives (P18-T04).
 *
 * Pages call `registerPrimitives()` once; a page that only needs one primitive
 * can call `registerPrimitive("ga-field")` instead, so a journey never ships
 * behaviour it does not use (docs/FRONTEND.md section 2).
 *
 * Decision recorded for this microtask: **no dialog primitive is added.**
 * No journey of the minimal web harness opens a modal, and the focus return it
 * would need is already covered here by the error summary (focus moves to the
 * field the user must fix) and by the toast (focus returns to where the user
 * was). Introducing a focus-trap without a caller would be untested code, so
 * the dialog arrives with the first journey that really needs one.
 *
 * Browser proofs owed to the end-to-end tooling (P18-T07): the keyboard and
 * zoom behaviour of these primitives is authored here and checked at the level
 * of decisions and stylesheets, but a real browser must still press Tab and
 * Escape, zoom to 200% and emulate reduced motion. `make test-e2e` does not
 * exist yet, so this microtask does not claim those proofs.
 */
import { GaBusyElement } from "./busy.js";
import { GaErrorSummaryElement } from "./error-summary.js";
import { GaFieldElement } from "./field.js";
import { GaToastElement } from "./toast.js";

/** Tag name of each primitive, with the element that implements it. */
const PRIMITIVES: ReadonlyMap<string, CustomElementConstructor> = new Map<string, CustomElementConstructor>([
  ["ga-field", GaFieldElement],
  ["ga-error-summary", GaErrorSummaryElement],
  ["ga-busy", GaBusyElement],
  ["ga-toast", GaToastElement],
]);

/** Tag names of every declared primitive. */
export function primitiveTags(): readonly string[] {
  return [...PRIMITIVES.keys()];
}

/**
 * Defines one primitive in the given registry. Defining a tag twice throws in
 * the platform, so a second call is a no-op and the page stays reloadable.
 */
export function registerPrimitive(tag: string, registry: CustomElementRegistry = customElements): void {
  const constructor = PRIMITIVES.get(tag);
  if (constructor === undefined) {
    throw new TypeError(`ga-primitives: unknown primitive ${JSON.stringify(tag)}`);
  }
  if (registry.get(tag) === undefined) {
    registry.define(tag, constructor);
  }
}

/** Defines every primitive once. */
export function registerPrimitives(registry: CustomElementRegistry = customElements): void {
  for (const tag of PRIMITIVES.keys()) {
    registerPrimitive(tag, registry);
  }
}
