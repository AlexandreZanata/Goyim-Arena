/**
 * The environment the harness prepares (P18-T07).
 *
 * Every value is read at call time and refused by name when it is missing. A
 * journey that silently defaulted its address, its account or its Arena would
 * test a different database than the one the harness seeded, and the failure
 * would look like a product defect instead of a missing variable.
 */

/**
 * required returns one variable, refusing an empty or blank value by name.
 *
 * It is exported because the language assertions of `locales.js` read the
 * language the harness seeded the arena with, and a missing variable there must
 * fail by name exactly like a missing address does: a journey that defaulted
 * the content language would compare the document against its own guess.
 */
export function required(name) {
  const value = process.env[name];
  if (value === undefined || value.trim() === "") {
    throw new Error(
      `${name} is required: tools/e2e/harness.sh prepares it when it starts the server and seeds the journeys`,
    );
  }
  return value;
}

/**
 * accountEnvironment is what the account journey needs: the server it drives,
 * the directory the delivered messages are written to, and the run identifier
 * that keeps the addresses of one run apart from the next.
 */
export function accountEnvironment() {
  return {
    baseURL: required("ARENA_E2E_BASE_URL"),
    sinkDirectory: required("ARENA_EMAIL_SINK_DIR"),
    runID: required("ARENA_E2E_RUN_ID"),
  };
}

/**
 * contentLanguage is the language the arena of the run was seeded with. It is
 * the language an Arena document declares (I18N_STANDARD.md §7), and the
 * journeys assert it in every interface locale to prove that reading an Arena
 * in another language never changes the Arena.
 */
export function contentLanguage() {
  return required("ARENA_E2E_CONTENT_LANGUAGE");
}

/**
 * participationEnvironment is what the Arena journey needs on top of that: the
 * Arena the harness published and the two accounts it credited with INK.
 *
 * Two accounts, because the domain refuses attributing an argument to its own
 * author: the influence one person records must have been written by someone
 * else, and a journey with a single account could not reach that transition.
 */
export function participationEnvironment() {
  return {
    ...accountEnvironment(),
    arenaSlug: required("ARENA_E2E_ARENA_SLUG"),
    participantEmail: required("ARENA_E2E_PARTICIPANT_EMAIL"),
    participantPassword: required("ARENA_E2E_PARTICIPANT_PASSWORD"),
    partnerEmail: required("ARENA_E2E_PARTNER_EMAIL"),
    partnerPassword: required("ARENA_E2E_PARTNER_PASSWORD"),
  };
}

/**
 * arenaSlugFor returns the Arena of one interface locale (P20-T09).
 *
 * The journeys are driven once per locale, against one server and one database,
 * and a position is immutable and an argument is published once: a journey that
 * reused the Arena of the previous locale would start from a state the previous
 * run left behind, and "the journeys pass in both languages" would depend on
 * the order they ran in. The harness therefore publishes one Arena per locale
 * and names them in one variable; a locale without an Arena is refused by name
 * instead of silently reusing another one.
 */
export function arenaSlugFor(locale) {
  const document = required("ARENA_E2E_ARENAS");
  let arenas;
  try {
    arenas = JSON.parse(document);
  } catch (error) {
    throw new Error(`ARENA_E2E_ARENAS is not a JSON object of locale to Arena slug: ${error.message}`);
  }
  const slug = arenas[locale];
  if (typeof slug !== "string" || slug.trim() === "") {
    throw new Error(
      `ARENA_E2E_ARENAS declares no Arena for the locale ${locale}: tools/e2e/harness.sh seeds one per locale of the suite`,
    );
  }
  return slug;
}
