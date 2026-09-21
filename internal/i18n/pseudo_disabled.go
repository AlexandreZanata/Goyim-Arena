//go:build !pseudolocale

package i18n

// PseudoEnabled reports whether this build carries the pseudo-locale catalog.
//
// The default build does not: the tag is what a CI or development build asks
// for, and the delivered binary is never built with it. The unit suite asserts
// both directions, so a build that lost the tag (or one that gained it by
// accident) fails instead of shipping a locale nobody asked for.
const PseudoEnabled = false
