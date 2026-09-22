/**
 * The account journey in a real browser (P18-T07), in every interface locale
 * the product ships (P20-T09).
 *
 * What this journey is for: it is the only place where the whole account path
 * is exercised the way a person exercises it — registration, the confirmation
 * code that arrives *outside* the process that answered the form, the session
 * cookie, and the sign-out that ends it. The server-side tests of the phase run
 * in-process with an in-memory sink, which cannot prove that a journey beside
 * the server can complete it; this one can, because the code it consumes was
 * written to a directory by another process.
 *
 * Why one test per locale instead of one test that loops: the runner reports
 * what failed, and `the account journey (en-US)` failing is a different piece
 * of news from the same journey failing in the default locale. The context is
 * created by the journey rather than taken from the `page` fixture, because the
 * locale is a property of the context: a journey that asserted a language on a
 * page whose context never asked for it would prove nothing.
 */
import { expect, test } from "@playwright/test";
import { confirm, register, sessionCookie, signIn, signOut } from "../support/account.js";
import { accountEnvironment } from "../support/environment.js";
import { JOURNEY_LOCALES } from "../support/locales.js";
import { VERIFICATION, lastTokenFor } from "../support/sink.js";

for (const locale of JOURNEY_LOCALES) {
  test(`an account is registered, confirmed through the delivered message, signed in and signed out (${locale})`, async ({
    browser,
  }) => {
    const environment = accountEnvironment();
    // One address per locale, in lower case: the address identifies the run of
    // one locale, and the sink is read back by the same string the server
    // stored — a capital letter would make the reader look for an address that
    // was never written.
    const email = `e2e-account-${locale.toLowerCase()}-${environment.runID}@example.test`;
    const password = "correct horse battery staple";

    const context = await browser.newContext({ locale });
    try {
      const page = await context.newPage();

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
    } finally {
      await context.close();
    }
  });
}
