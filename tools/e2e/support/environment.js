/**
 * The environment the harness prepares (P18-T07).
 *
 * Every value is read at call time and refused by name when it is missing. A
 * journey that silently defaulted its address, its account or its Arena would
 * test a different database than the one the harness seeded, and the failure
 * would look like a product defect instead of a missing variable.
 */

/** required returns one variable, refusing an empty or blank value by name. */
function required(name) {
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
