/**
 * Runner configuration of the browser journeys (P18-T07).
 *
 * Three decisions are deliberate and are the reason this file is short:
 *
 *   - the server is not started here. The harness (`harness.sh`) composes the
 *     process, the database and the email sink directory, and names them in the
 *     environment; a runner that started its own server would hide the
 *     composition it is supposed to exercise.
 *   - there are no retries and no parallel workers. The journeys share one
 *     server, one database and one sink directory, and a journey that passes
 *     only on the second attempt is a defect that a retry would hide.
 *   - a focused test (`test.only`) fails the run instead of silently shortening
 *     it.
 */
import { defineConfig } from "@playwright/test";

const baseURL = process.env.ARENA_E2E_BASE_URL;
if (baseURL === undefined || baseURL.trim() === "") {
  throw new Error(
    "ARENA_E2E_BASE_URL is required: run the journeys through tools/e2e/harness.sh, which starts the server they drive",
  );
}

export default defineConfig({
  testDir: "./specs",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: true,
  reporter: [["list"]],
  timeout: 30_000,
  expect: { timeout: 5_000 },
  use: {
    baseURL,
    // One locale for the whole run, so a page rendered for the default
    // Accept-Language of the browser never changes the assertions.
    locale: process.env.ARENA_E2E_LOCALE ?? "pt-BR",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
