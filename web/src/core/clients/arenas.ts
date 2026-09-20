/**
 * Arenas client (P18-T03; journey P18-T06).
 *
 * Public reads only. Everything here is a safe method, so the core applies
 * its retry policy, and the HTTP cache keeps the ETags the backend sends.
 */
import type { ArenaFeed, PublicArena, SearchArenaPage } from "../../contracts/generated.js";
import type { HttpCore } from "../http.js";

/** Feed filters of `GET /api/v1/arenas`; cursor pages stay opaque. */
export interface ArenaFeedQuery {
  readonly language?: string;
  readonly category?: string;
  readonly status?: string;
  readonly cursor?: string;
  readonly limit?: number;
}

/** Search filters of `GET /api/v1/search/arenas`. */
export interface ArenaSearchQuery {
  readonly q: string;
  readonly language?: string;
  readonly cursor?: string;
  readonly limit?: number;
}

/** Read operations the arena pages need. */
export interface ArenasClient {
  feed(query?: ArenaFeedQuery): Promise<ArenaFeed>;
  bySlug(slug: string): Promise<PublicArena>;
  search(query: ArenaSearchQuery): Promise<SearchArenaPage>;
}

const ARENAS_PATH = "/api/v1/arenas";
const SEARCH_PATH = "/api/v1/search/arenas";

/** createArenasClient binds the public Arena reads to a shared core. */
export function createArenasClient(core: HttpCore): ArenasClient {
  return {
    feed: (query?: ArenaFeedQuery): Promise<ArenaFeed> =>
      core.request<ArenaFeed>({
        method: "GET",
        path: ARENAS_PATH,
        ...(query === undefined
          ? {}
          : { query: { language: query.language, category: query.category, status: query.status, cursor: query.cursor, limit: query.limit } }),
      }),

    bySlug: (slug: string): Promise<PublicArena> =>
      core.request<PublicArena>({ method: "GET", path: `${ARENAS_PATH}/${encodeURIComponent(slug)}` }),

    search: (query: ArenaSearchQuery): Promise<SearchArenaPage> =>
      core.request<SearchArenaPage>({
        method: "GET",
        path: SEARCH_PATH,
        query: { q: query.q, language: query.language, cursor: query.cursor, limit: query.limit },
      }),
  };
}
