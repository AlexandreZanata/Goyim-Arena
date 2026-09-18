package domain

// ExportSchemaVersion pins the schema of the public Arena export
// (P14-T04; docs/TRANSPARENCY.md §7). The version travels in every page of
// the document, so clients parse a known shape and a future incompatible
// layout must bump this constant instead of silently reinterpreting v1.
const ExportSchemaVersion = 1
