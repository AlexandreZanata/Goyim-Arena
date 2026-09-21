/**
 * Reader of the directory email sink (P18-T07).
 *
 * This is the other side of the contract `internal/identity/adapters/emailsink`
 * documents, and it is the reason the journeys can be completed without a
 * provider: the server writes one file per delivered message, the harness reads
 * the code it carries. Migration of the contract is a change on both sides —
 * the adapter tests assert the document keys this module reads.
 */
import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";

/** The kind of message a journey waits for, as the adapter names it. */
export const VERIFICATION = "verification";

/**
 * deliveredMessages returns the messages of a sink directory in delivery order.
 * A name that starts with a dot is a file the writer is still filling, never a
 * message; the adapter renames its documents into place, and this filter is the
 * reader's half of that agreement.
 */
export async function deliveredMessages(directory) {
  const names = (await readdir(directory))
    .filter((name) => !name.startsWith("."))
    .sort();

  const messages = [];
  for (const name of names) {
    const document = JSON.parse(await readFile(join(directory, name), "utf8"));
    messages.push({ ...document, file: name });
  }
  return messages;
}

/**
 * lastTokenFor waits for the most recent message of one kind addressed to one
 * address and returns the code it carries.
 *
 * Waiting, instead of reading once, because the file is written while the page
 * that triggered it is still being answered: the journey is allowed to look for
 * the message immediately after submitting the form.
 */
export async function lastTokenFor(directory, { email, kind = VERIFICATION, timeoutMs = 10_000 }) {
  const deadline = Date.now() + timeoutMs;

  for (;;) {
    const messages = await deliveredMessages(directory);
    const matching = messages.filter((message) => message.kind === kind && message.email === email);
    const last = matching.at(-1);
    if (last !== undefined && typeof last.token === "string" && last.token !== "") {
      return last.token;
    }

    if (Date.now() >= deadline) {
      throw new Error(
        `no ${kind} message for ${email} arrived in ${directory} within ${timeoutMs}ms; ` +
          `the directory holds ${messages.length} message(s)`,
      );
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}
