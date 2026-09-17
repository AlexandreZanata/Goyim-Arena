// Package text owns Unicode text measurement for the backend (ADR-013).
// It is the single owner of the approved grapheme segmentation dependency
// and exposes plain Go values: supplier types never cross this boundary.
//
// Domain and application layers stay standard-library only and consume this
// capability through small ports composed at bootstrap (ADR-011, ADR-013).
package text

import "github.com/rivo/uniseg"

// GraphemeCount returns the number of extended grapheme clusters
// (user-perceived characters) in value, per Unicode UAX #29: combining
// marks, ZWJ emoji sequences, skin-tone modifiers and flag pairs count as
// one cluster each. Spaces, punctuation and line breaks are clusters of
// their own and therefore count.
func GraphemeCount(value string) int {
	return uniseg.GraphemeClusterCount(value)
}
