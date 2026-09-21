// The entry module of one public page: the document loads this one file and the
// browser follows its relative specifiers, which is exactly the closure the
// page budget has to measure.
import { request } from "../core/http.js";

export function loadArena(slug) {
    // A comment that names innerHTML and https://example.invalid/on-purpose
    // proves the scans read code and not documentation.
    return request(`/api/v1/arenas/${slug}`);
}
