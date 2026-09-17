package domain

// ReplyPolicy is the versioned depth policy of argument replies. The MVP
// accepts one recursion level (REQ-ARG-02): a top-level argument receives
// replies, a reply receives none. The values are configuration injected at
// bootstrap, never content judgments.
type ReplyPolicy struct {
	// Version identifies the configuration revision the limit came from.
	Version string
	// MaxDepth is the deepest reply depth accepted, counting the top-level
	// argument as depth 0.
	MaxDepth int
}

// DefaultReplyPolicy returns the initial depth policy of the MVP.
func DefaultReplyPolicy() ReplyPolicy {
	return ReplyPolicy{
		Version:  "2026-09",
		MaxDepth: 1,
	}
}

// IsValid reports whether the policy is internally coherent.
func (p ReplyPolicy) IsValid() bool {
	return p.Version != "" && p.MaxDepth >= 1
}

// AllowsChildDepth reports whether a reply at the given depth may exist:
// the depth counts the top-level argument as 0, so a reply to it is depth 1.
func (p ReplyPolicy) AllowsChildDepth(depth int) bool {
	return depth >= 1 && depth <= p.MaxDepth
}
