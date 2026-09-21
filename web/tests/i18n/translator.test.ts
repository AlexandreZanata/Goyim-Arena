/**
 * Tests of the typed translator (P18-T09).
 *
 * The generated catalog is what a page actually renders, so most of these tests
 * run against it — pt-BR and en-US side by side. The cases the catalog cannot
 * express yet (plural variants, a locale that lacks a message, a catalog whose
 * text disagrees with its declaration) are exercised through the injected
 * catalog, which is the same door the pseudo-locale of the CI gate will use.
 */
import assert from "node:assert/strict";
import { test } from "node:test";

import { messagePlaceholders, messages } from "../../src/i18n/generated.js";
import {
  MissingMessageError,
  MissingPlaceholderError,
  createTranslator,
  selectCatalog,
} from "../../src/i18n/translator.js";
import type { CatalogSource, Translator } from "../../src/i18n/translator.js";
import type { MessageKey } from "../../src/i18n/generated.js";

/** The generated catalog, as a source a translator can be created from. */
const generated: CatalogSource = { messages, placeholders: messagePlaceholders };

/** A JavaScript view of a translator: the compiler's checks are not the only ones. */
interface UntypedTranslator {
  translate(key: string, values?: Record<string, unknown>): string;
  plural(key: string, count: number, values?: Record<string, unknown>): string;
}

function fromJavaScript(translator: Translator): UntypedTranslator {
  return translator as unknown as UntypedTranslator;
}

test("a message is rendered in the locale of the translator", () => {
  assert.equal(createTranslator("pt-BR").translate("auth.login.submit"), "Entrar");
  assert.equal(createTranslator("en-US").translate("auth.login.submit"), "Sign in");
  assert.equal(createTranslator("en-US").translate("arenas.participation.choice.agree"), "Agree");
});

test("a number value is grouped for the locale of the page", () => {
  assert.equal(
    createTranslator("pt-BR").translate("arenas.participation.aggregate.total", { total: 2431 }),
    "Participantes elegíveis: 2.431",
  );
  assert.equal(
    createTranslator("en-US").translate("arenas.participation.aggregate.total", { total: 2431 }),
    "Eligible participants: 2,431",
  );
});

test("text values are inserted as text", () => {
  assert.equal(
    createTranslator("pt-BR").translate("arenas.participation.attribution.option", {
      relation: "A favor",
      excerpt: "O imposto precisa mudar",
    }),
    "A favor: O imposto precisa mudar",
  );
});

test("a value that looks like a placeholder is not expanded again", () => {
  // The single substitution pass is the property under test: a value carrying
  // braces is data, so `{total}` — a placeholder of another message — stays a
  // literal brace and never becomes a second round of interpolation. Markup in
  // a value is data too: this module composes text, and the DOM boundary writes
  // it with textContent (asserted by the architecture gate).
  const hostile = '{total}<img src=x onerror="alert(1)">';
  const rendered = createTranslator("pt-BR").translate("auth.errors.weak_password", { min: hostile });

  assert.equal(rendered, `A senha deve ter pelo menos ${hostile} caracteres.`);
});

test("a placeholder the message declares and nobody supplied is refused", () => {
  const translator = createTranslator("pt-BR");
  const untyped = fromJavaScript(translator);

  assert.throws(() => untyped.translate("auth.errors.weak_password", {}), MissingPlaceholderError);
  assert.throws(() => untyped.translate("auth.errors.weak_password"), MissingPlaceholderError);
});

test("a message whose text disagrees with its declaration is refused, not left with a hole", () => {
  // The declaration is the authority, and the substitution pass is the backstop:
  // here the declaration forgot `{min}`, so the check on the declaration passes
  // and the hole is caught while substituting.
  const inconsistent: CatalogSource = {
    messages: { "pt-BR": { "auth.field.password_hint": "Mínimo de {min} caracteres." } },
    placeholders: { "auth.field.password_hint": [] },
  };
  const untyped = fromJavaScript(createTranslator("pt-BR", { catalog: inconsistent }));

  assert.throws(() => untyped.translate("auth.field.password_hint"), MissingPlaceholderError);
});

test("a key the catalog does not have is refused and never rendered", () => {
  const translator = createTranslator("pt-BR");
  const untyped = fromJavaScript(translator);

  assert.throws(() => untyped.translate("auth.nope.nope"), MissingMessageError);
  assert.ok(translator.has("auth.login.submit"));
  assert.ok(!translator.has("auth.nope.nope" as MessageKey));
});

test("the fallback locale answers when the requested one has no message", () => {
  const portugueseOnly: CatalogSource = {
    messages: { "pt-BR": { "auth.login.submit": "Entrar" } },
    placeholders: { "auth.login.submit": [] },
  };
  assert.equal(createTranslator("en-US", { catalog: portugueseOnly }).translate("auth.login.submit"), "Entrar");
  assert.throws(
    () => createTranslator("en-US", { catalog: portugueseOnly, fallbackLocale: null }).translate("auth.login.submit"),
    MissingMessageError,
  );
});

test("a namespace restricts what a translator can render", () => {
  const account = createTranslator("pt-BR", { namespaces: ["auth"] });

  assert.deepEqual(account.namespaces, ["auth"]);
  assert.ok(account.has("auth.login.submit"));
  assert.ok(!account.has("email.greeting"), "a key of another namespace is not in this catalog");
  assert.throws(() => account.translate("email.greeting"), MissingMessageError);
  assert.equal(account.translate("auth.login.submit"), "Entrar");
});

test("selectCatalog returns one locale and one namespace, and nothing else", () => {
  const auth = selectCatalog(generated, "en-US", ["auth"]);
  const keys = Object.keys(auth);

  assert.ok(keys.length > 0);
  assert.ok(keys.every((key) => key.startsWith("auth.")), "a namespace selection carries only its own keys");
  assert.equal(auth["auth.login.submit"], messages["en-US"]?.["auth.login.submit"]);
  assert.equal(auth["arenas.participation.brand"], undefined);
  assert.deepEqual(Object.keys(selectCatalog(generated, "en-US", [])), []);
});

test("plural selects the variant the category of the locale names", () => {
  // The base key is one the generated catalog declares; the injected catalog
  // adds the structured variants the convention defines.
  const replies = "arenas.participation.arguments.replies";
  const variants: CatalogSource = {
    messages: {
      "pt-BR": { [`${replies}.one`]: "{count} resposta", [`${replies}.other`]: "{count} respostas" },
      "en-US": { [`${replies}.one`]: "{count} reply", [`${replies}.other`]: "{count} replies" },
    },
    placeholders: { [`${replies}.one`]: ["count"], [`${replies}.other`]: ["count"] },
  };

  const pt = createTranslator("pt-BR", { catalog: variants });
  const en = createTranslator("en-US", { catalog: variants });

  assert.equal(pt.plural(replies, 1), "1 resposta");
  assert.equal(pt.plural(replies, 2), "2 respostas");
  // Zero is `one` in Portuguese and `other` in English: the whole point of
  // letting CLDR select instead of comparing the count to one.
  assert.equal(pt.plural(replies, 0), "0 resposta");
  assert.equal(en.plural(replies, 0), "0 replies");
  assert.equal(en.plural(replies, 1), "1 reply");
});

test("a message without variants is used as it is", () => {
  const replies = "arenas.participation.arguments.replies";

  assert.equal(createTranslator("pt-BR").plural(replies, 3), "Respostas: 3");
  assert.equal(createTranslator("en-US").plural(replies, 1), "Replies: 1");
});

test("plural supplies the count itself, and refuses a key it cannot render", () => {
  const untyped = fromJavaScript(createTranslator("pt-BR"));

  // A JavaScript caller passing its own count does not change what is rendered:
  // the count argument is the one the variant interpolates.
  assert.equal(untyped.plural("arenas.participation.arguments.replies", 2, { count: 99 }), "Respostas: 2");
  assert.throws(() => untyped.plural("auth.nope.nope", 1), MissingMessageError);
});
