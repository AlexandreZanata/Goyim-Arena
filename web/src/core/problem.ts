/**
 * RFC 9457 Problem Details decoding for the native HTTP core (P18-T03).
 *
 * The API answers failures with `application/problem+json` and a stable
 * `code`. Clients depend on that code (and on the presentation `kind` this
 * module derives from the status); they never render the wire `title` or
 * `detail` directly, because localized UI text lives in the catalogs
 * (I18N_STANDARD.md section 5 and docs/FRONTEND.md section 6).
 */
import type { Problem } from "../contracts/generated.js";

/** Error media type of the versioned contract. */
export const PROBLEM_MEDIA_TYPE = "application/problem+json";

/**
 * Presentation category catalogued in `locales/<locale>/errors.json`.
 * Every kind has `title` and `detail` messages in each supported locale.
 */
export type ErrorKind =
  | "validation"
  | "unauthorized"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "rate_limited"
  | "internal";

/**
 * Stable codes the client itself produces when the failure never reached the
 * application: they are never translated, only mapped to a kind or handled.
 */
export const NETWORK_ERROR_CODE = "network_error";
export const INVALID_RESPONSE_CODE = "invalid_response";
export const UNEXPECTED_RESPONSE_CODE = "unexpected_response";
export const TIMEOUT_ERROR_CODE = "timeout";
export const ABORTED_ERROR_CODE = "aborted";

/** Codes with no server counterpart, grouped for callers and documentation. */
export const CLIENT_ERROR_CODES: readonly string[] = [
  NETWORK_ERROR_CODE,
  INVALID_RESPONSE_CODE,
  UNEXPECTED_RESPONSE_CODE,
  TIMEOUT_ERROR_CODE,
  ABORTED_ERROR_CODE,
];

/**
 * errorKindForStatus maps an HTTP status onto the catalogued presentation
 * category. It is deliberately total: an unmapped status still resolves to a
 * kind the catalog can render, never to a raw status the UI would show.
 */
export function errorKindForStatus(status: number): ErrorKind {
  switch (status) {
    case 401:
      return "unauthorized";
    case 403:
      return "forbidden";
    case 404:
    case 410:
      return "not_found";
    case 409:
    case 412:
      return "conflict";
    case 429:
      return "rate_limited";
    default:
      break;
  }
  if (status >= 500) {
    return "internal";
  }
  if (status >= 400) {
    return "validation";
  }
  return "internal";
}

/**
 * isProblem reports whether a decoded JSON value is a complete RFC 9457
 * Problem as this contract defines it.
 */
export function isProblem(value: unknown): value is Problem {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate["type"] === "string" &&
    typeof candidate["title"] === "string" &&
    typeof candidate["code"] === "string" &&
    typeof candidate["status"] === "number"
  );
}

/**
 * parseProblemBody decodes a response body into a Problem. It returns null
 * instead of throwing: an unexpected body must surface as a client code, not
 * as a parse exception that hides the HTTP status.
 */
export function parseProblemBody(body: string): Problem | null {
  if (body.trim() === "") {
    return null;
  }
  let decoded: unknown;
  try {
    decoded = JSON.parse(body);
  } catch {
    return null;
  }
  return isProblem(decoded) ? decoded : null;
}

/** Every field of an ApiError is explicit: callers branch on data, not prose. */
export interface ApiErrorInit {
  readonly code: string;
  readonly kind: ErrorKind | null;
  readonly status: number | null;
  readonly problem: Problem | null;
  readonly requestId: string | null;
  readonly retryAfterSeconds: number | null;
  readonly retryable: boolean;
  readonly aborted: boolean;
  readonly attempts: number;
  readonly cause?: unknown;
}

/**
 * ApiError is the single failure type of the HTTP core. `message` is the
 * stable code (never shown to the user); `kind` selects the catalogued
 * presentation category and `problem` keeps the wire detail for diagnostics.
 */
export class ApiError extends Error {
  readonly code: string;
  readonly kind: ErrorKind | null;
  readonly status: number | null;
  readonly problem: Problem | null;
  readonly requestId: string | null;
  readonly retryAfterSeconds: number | null;
  readonly retryable: boolean;
  readonly aborted: boolean;
  readonly attempts: number;

  constructor(init: ApiErrorInit) {
    super(init.code, init.cause === undefined ? undefined : { cause: init.cause });
    this.name = "ApiError";
    this.code = init.code;
    this.kind = init.kind;
    this.status = init.status;
    this.problem = init.problem;
    this.requestId = init.requestId;
    this.retryAfterSeconds = init.retryAfterSeconds;
    this.retryable = init.retryable;
    this.aborted = init.aborted;
    this.attempts = init.attempts;
  }
}

/**
 * parseRetryAfter reads `Retry-After` as whole seconds, per RFC 9110. HTTP
 * dates are also accepted; a past date means "retry immediately". Invalid
 * values are ignored so a malformed header never blocks recovery.
 */
export function parseRetryAfter(header: string | null, nowMs: number): number | null {
  if (header === null) {
    return null;
  }
  const trimmed = header.trim();
  if (trimmed === "") {
    return null;
  }
  if (/^\d+$/.test(trimmed)) {
    const seconds = Number.parseInt(trimmed, 10);
    return Number.isSafeInteger(seconds) ? seconds : null;
  }
  // Only the IMF-fixdate form is parsed as a date: `Date.parse` would happily
  // read junk like "-5" as a year, and an invalid delay must be ignored
  // instead of silently turning into "retry now".
  if (!/^[A-Za-z]{3},\s/.test(trimmed)) {
    return null;
  }
  const date = Date.parse(trimmed);
  if (Number.isNaN(date)) {
    return null;
  }
  return Math.max(0, Math.ceil((date - nowMs) / 1000));
}
