/**
 * Arguments client (P18-T03; journey P18-T06).
 *
 * Public reads stay cache-friendly; publishing and replying are unsafe, so
 * they are retried only because the core attaches the `Idempotency-Key` the
 * contract requires for those two operations.
 */
import type {
  Argument,
  ArgumentMutationResult,
  ArgumentPage,
  ArgumentPublishRequest,
} from "../../contracts/generated.js";
import type { HttpCore } from "../http.js";

/** Filters of `GET /api/v1/arenas/{id}/arguments`; `relation` is required. */
export interface ArenaArgumentsQuery {
  readonly relation: string;
  readonly cursor?: string;
  readonly limit?: number;
}

/** Cursor page of replies. */
export interface ArgumentRepliesQuery {
  readonly cursor?: string;
  readonly limit?: number;
}

/** Read and write operations the Arena argument list needs. */
export interface ArgumentsClient {
  list(arenaId: string, query: ArenaArgumentsQuery): Promise<ArgumentPage>;
  get(argumentId: string): Promise<Argument>;
  replies(argumentId: string, query?: ArgumentRepliesQuery): Promise<ArgumentPage>;
  publish(arenaId: string, input: ArgumentPublishRequest): Promise<ArgumentMutationResult>;
  reply(arenaId: string, argumentId: string, input: ArgumentPublishRequest): Promise<ArgumentMutationResult>;
}

const ARENAS_PATH = "/api/v1/arenas";
const ARGUMENTS_PATH = "/api/v1/arguments";
const ME_ARENAS_PATH = "/api/v1/me/arenas";

/** Two attempts: enough to survive a dropped response, never a hammer. */
const MUTATION_RETRY = { maxAttempts: 2 } as const;

/** createArgumentsClient binds the argument operations to a shared core. */
export function createArgumentsClient(core: HttpCore): ArgumentsClient {
  return {
    list: (arenaId: string, query: ArenaArgumentsQuery): Promise<ArgumentPage> =>
      core.request<ArgumentPage>({
        method: "GET",
        path: `${ARENAS_PATH}/${encodeURIComponent(arenaId)}/arguments`,
        query: { relation: query.relation, cursor: query.cursor, limit: query.limit },
      }),

    get: (argumentId: string): Promise<Argument> =>
      core.request<Argument>({ method: "GET", path: `${ARGUMENTS_PATH}/${encodeURIComponent(argumentId)}` }),

    replies: (argumentId: string, query?: ArgumentRepliesQuery): Promise<ArgumentPage> =>
      core.request<ArgumentPage>({
        method: "GET",
        path: `${ARGUMENTS_PATH}/${encodeURIComponent(argumentId)}/replies`,
        ...(query === undefined ? {} : { query: { cursor: query.cursor, limit: query.limit } }),
      }),

    publish: (arenaId: string, input: ArgumentPublishRequest): Promise<ArgumentMutationResult> =>
      core.request<ArgumentMutationResult>({
        method: "POST",
        path: `${ME_ARENAS_PATH}/${encodeURIComponent(arenaId)}/arguments`,
        body: input,
        retry: MUTATION_RETRY,
      }),

    reply: (arenaId: string, argumentId: string, input: ArgumentPublishRequest): Promise<ArgumentMutationResult> =>
      core.request<ArgumentMutationResult>({
        method: "POST",
        path: `${ME_ARENAS_PATH}/${encodeURIComponent(arenaId)}/arguments/${encodeURIComponent(argumentId)}/replies`,
        body: input,
        retry: MUTATION_RETRY,
      }),
  };
}
