/**
 * The Arena participation journey in a real browser (P18-T07).
 *
 * The four transitions of the journey — confirm the initial position, publish
 * an argument, change the position, record what influenced the change — are
 * submitted by the pages themselves: every form has a real action and a real
 * CSRF field, and the assertions are about the answers the server gave, not
 * about the wording of the catalogues.
 *
 * Two accounts, in two browser contexts, because the domain refuses
 * attributing an argument to its own author: the influence the participant
 * records has to have been written by someone else, and a journey driven by a
 * single account could never reach that transition.
 *
 * The publication charges INK. That a ledger line was written is proved where
 * the database is, by the composition test of the phase; what this journey
 * proves is that the page a browser gets is the page that can spend it.
 */
import { expect, test } from "@playwright/test";
import { signIn } from "../support/account.js";
import { participationEnvironment } from "../support/environment.js";

/**
 * The argument the partner publishes. The text is unique to this journey so
 * the attribution option can be found by what it says: the identifier of an
 * argument is not something a page hands to a person, and a journey that
 * selected a checkbox by position would pass while selecting the wrong one.
 */
const PARTNER_ARGUMENT = "O argumento do parceiro sustenta a verificacao publica dos fatos.";

/** The argument the participant publishes, which the page must list afterwards. */
const PARTICIPANT_ARGUMENT = "O argumento do participante discorda do custo por grafema.";

test("the Arena journey confirms a position, publishes, changes it and credits what influenced the change", async ({
  browser,
}) => {
  const environment = participationEnvironment();
  const slug = environment.arenaSlug;

  const positionForm = (page) => page.locator(`form[action="/arenas/${slug}/position"]`);
  const changeForm = (page) => page.locator(`form[action="/arenas/${slug}/position/change"]`);
  const publishForm = (page) => page.locator(`form[action="/arenas/${slug}/arguments"]`);
  const attributionForm = (page) => page.locator(`form[action="/arenas/${slug}/attributions"]`);

  // The other author, in a context of its own: it confirms a position (the
  // publication form only exists once a person has one) and publishes the
  // argument the participant will later credit.
  const authorContext = await browser.newContext();
  try {
    const author = await authorContext.newPage();
    await signIn(author, { email: environment.partnerEmail, password: environment.partnerPassword });
    await author.goto(`/arenas/${slug}`);

    await positionForm(author).locator('input[name="position"][value="undecided"]').check();
    await Promise.all([
      author.waitForURL(/ok=position_confirmed/),
      positionForm(author).locator('button[type="submit"]').click(),
    ]);

    await publishForm(author).locator('input[name="relation"][value="support"]').check();
    await publishForm(author).locator('textarea[name="content"]').fill(PARTNER_ARGUMENT);
    await Promise.all([
      author.waitForURL(/ok=argument_published/),
      publishForm(author).locator('button[type="submit"]').click(),
    ]);
  } finally {
    await authorContext.close();
  }

  const participantContext = await browser.newContext();
  try {
    const participant = await participantContext.newPage();
    await signIn(participant, { email: environment.participantEmail, password: environment.participantPassword });
    await participant.goto(`/arenas/${slug}`);

    // The page lists the arguments of the Arena before anything is submitted.
    await expect(participant.getByText(PARTNER_ARGUMENT)).toBeVisible();

    // 1. The initial position. It is immutable, so it is submitted once and
    // the page that comes back carries the notice of the transition.
    await positionForm(participant).locator('input[name="position"][value="agree"]').check();
    await Promise.all([
      participant.waitForURL(/ok=position_confirmed/),
      positionForm(participant).locator('button[type="submit"]').click(),
    ]);
    await expect(participant.locator("ga-toast")).toBeVisible();

    // 2. The publication, the transition that charges INK. The page lists the
    // argument it just recorded.
    await publishForm(participant).locator('input[name="relation"][value="oppose"]').check();
    await publishForm(participant).locator('textarea[name="content"]').fill(PARTICIPANT_ARGUMENT);
    await Promise.all([
      participant.waitForURL(/ok=argument_published/),
      publishForm(participant).locator('button[type="submit"]').click(),
    ]);
    await expect(participant.getByText(PARTICIPANT_ARGUMENT)).toBeVisible();

    // 3. The change. The form starts on the stored position, so the page that
    // came back is itself the evidence that the change was recorded.
    await changeForm(participant).locator('input[name="position"][value="disagree"]').check();
    await Promise.all([
      participant.waitForURL(/ok=position_changed/),
      changeForm(participant).locator('button[type="submit"]').click(),
    ]);
    await expect(changeForm(participant).locator('input[name="position"][value="disagree"]')).toBeChecked();

    // 4. The attribution: the form exists because there is a change to credit,
    // and the option it offers is the argument of the other author.
    await expect(attributionForm(participant)).toBeVisible();
    await attributionForm(participant).getByLabel(/argumento do parceiro/).check();
    await Promise.all([
      participant.waitForURL(/ok=attribution_recorded/),
      attributionForm(participant).locator('button[type="submit"]').click(),
    ]);
  } finally {
    await participantContext.close();
  }
});
