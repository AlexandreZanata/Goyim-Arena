/**
 * Tests of the Problem Details vocabulary (P18-T03): the presentation kind
 * must be total, decoding must never throw, and Retry-After must accept both
 * spellings of RFC 9110.
 */
import assert from "node:assert/strict";
import { test } from "node:test";

import { ApiError, errorKindForStatus, isProblem, parseProblemBody, parseRetryAfter } from "../../src/core/problem.js";
import type { ErrorKind } from "../../src/core/problem.js";

test("errorKindForStatus maps every status onto a catalogued kind", () => {
  const cases: readonly (readonly [number, ErrorKind])[] = [
    [400, "validation"],
    [401, "unauthorized"],
    [403, "forbidden"],
    [404, "not_found"],
    [405, "validation"],
    [409, "conflict"],
    [410, "not_found"],
    [412, "conflict"],
    [418, "validation"],
    [422, "validation"],
    [429, "rate_limited"],
    [451, "validation"],
    [500, "internal"],
    [502, "internal"],
    [503, "internal"],
    [504, "internal"],
    [100, "internal"],
    [200, "internal"],
    [301, "internal"],
  ];
  for (const [status, kind] of cases) {
    assert.equal(errorKindForStatus(status), kind, `status ${status}`);
  }
});

test("parseProblemBody accepts a complete Problem and rejects everything else", () => {
  const complete = { type: "about:blank", title: "conflict", status: 409, code: "position_conflict" };
  assert.deepEqual(parseProblemBody(JSON.stringify(complete)), complete);

  const withRequestId = { ...complete, request_id: "req-9" };
  assert.deepEqual(parseProblemBody(JSON.stringify(withRequestId)), withRequestId);

  assert.equal(parseProblemBody(""), null);
  assert.equal(parseProblemBody("   "), null);
  assert.equal(parseProblemBody("<html>oops</html>"), null);
  assert.equal(parseProblemBody("null"), null);
  assert.equal(parseProblemBody('"a string"'), null);
  assert.equal(parseProblemBody(JSON.stringify({ ...complete, code: 42 })), null);
  assert.equal(parseProblemBody(JSON.stringify({ type: "about:blank", title: "t" })), null);
});

test("isProblem requires the four stable fields", () => {
  assert.equal(isProblem({ type: "t", title: "t", status: 400, code: "c" }), true);
  assert.equal(isProblem({ type: "t", title: "t", status: 400 }), false);
  assert.equal(isProblem(null), false);
  assert.equal(isProblem([]), false);
});

test("parseRetryAfter reads seconds, HTTP dates and ignores garbage", () => {
  assert.equal(parseRetryAfter("120", 0), 120);
  assert.equal(parseRetryAfter("0", 0), 0);
  assert.equal(parseRetryAfter(" 7 ", 0), 7);

  const now = Date.parse("2026-09-20T12:00:00Z");
  assert.equal(parseRetryAfter("Sun, 20 Sep 2026 12:00:30 GMT", now), 30);
  assert.equal(parseRetryAfter("Sun, 20 Sep 2026 11:59:00 GMT", now), 0);

  assert.equal(parseRetryAfter(null, now), null);
  assert.equal(parseRetryAfter("", now), null);
  assert.equal(parseRetryAfter("soon", now), null);
  assert.equal(parseRetryAfter("-5", now), null);
});

test("ApiError carries data instead of prose", () => {
  const failure = new ApiError({
    code: "position_conflict",
    kind: "conflict",
    status: 409,
    problem: { type: "about:blank", title: "conflict", status: 409, code: "position_conflict" },
    requestId: "req-3",
    retryAfterSeconds: null,
    retryable: false,
    aborted: false,
    attempts: 1,
  });
  assert.equal(failure instanceof Error, true);
  assert.equal(failure instanceof ApiError, true);
  assert.equal(failure.name, "ApiError");
  assert.equal(failure.message, "position_conflict");
  assert.equal(failure.kind, "conflict");
  assert.equal(failure.status, 409);
  assert.equal(failure.problem?.code, "position_conflict");
  assert.equal(failure.requestId, "req-3");
  assert.equal(failure.retryable, false);
  assert.equal(failure.aborted, false);
  assert.equal(failure.attempts, 1);
});
