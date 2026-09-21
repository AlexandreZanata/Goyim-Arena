/**
 * The account journey in a real browser (P18-T07).
 *
 * What this journey is for: it is the only place where the whole account path
 * is exercised the way a person exercises it — registration, the confirmation
 * code that arrives *outside* the process that answered the form, the session
 * cookie, and the sign-out that ends it. The server-side tests of the phase run
 * in-process with an in-memory sink, which cannot prove that a journey beside
 * the server can complete it; this one can, because the code it consumes was
 * written to a directory by another process.
 */
import { expect, test } from "@playwright/test";
import { confirm, register, sessionCookie, signIn, signOut } from "../support/account.js";
import { accountEnvironment } from "../support/environment.js";
import { VERIFICATION, lastTokenFor } from "../support/sink.js";

test("an account is registered, confirmed through the delivered message, signed in and signed out", async ({ page }) => {
  const environment = accountEnvironment();
  const email = `e2e-account-${environment.runID}@example.test`;
  const password = "correct horse battery staple";

  await register(page, { email, password });

  // The message was delivered by the server to the sink directory: this
  // process holds no reference to the token generator, and reading the code
  // anyway is exactly what the directory sink exists for.
  const code = await lastTokenFor(environment.sinkDirectory, { email, kind: VERIFICATION });
  expect(code).not.toBe("");

  await confirm(page, code);

  await signIn(page, { email, password });

  // The session is what the browser now holds: an httpOnly cookie issued by
  // the security boundary, not a value any script could read.
  const session = await sessionCookie(page);
  expect(session).not.toBeNull();
  expect(session.httpOnly).toBe(true);

  await signOut(page);
  expect(await sessionCookie(page)).toBeNull();
});
