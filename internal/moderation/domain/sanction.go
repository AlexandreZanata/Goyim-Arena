package domain

// SanctionMatrix maps every moderation action onto the targets it may
// sanction (P13-T05). It complements the role matrix of policy.go: the
// role decides WHO may act, this matrix decides WHAT may be sanctioned.
// Both must allow, so a moderator can remove arguments but never suspend
// accounts, and an admin can suspend profiles but never close an argument.
//
// Projection effects live with the adapter in the same transaction as the
// audit event (the moderation_actions row). Actions without a projection
// effect are recorded only, with the scope carried by the justification:
//   - arena_close closes a published Arena;
//   - argument_remove removes a published or withdrawn argument;
//   - attribution_invalidate invalidates the valid attributions crediting
//     the targeted argument, preserving each decision triple;
//   - suspension and ban move an active account to suspended (ban is the
//     same projection without an expiry: indefinite until reversed);
//   - warning, link_hide, interaction_limit, no_action, preserve_legal and
//     position_invalidate record the decision without mutating a
//     projection: link and interaction controls have no dedicated columns
//     in the MVP, legal preservation is an operational marker, and
//     positions gain no invalidation marker until a later phase.
var sanctionTargets = map[Action]map[TargetType]bool{
	ActionNoAction: {
		TargetArena: true, TargetArgument: true, TargetProfile: true,
	},
	ActionWarning: {
		TargetArena: true, TargetArgument: true, TargetProfile: true,
	},
	ActionLinkHide: {
		TargetArena: true, TargetArgument: true,
	},
	ActionInteractionLimit: {
		TargetProfile: true,
	},
	ActionArgumentRemove: {
		TargetArgument: true,
	},
	ActionArenaClose: {
		TargetArena: true,
	},
	ActionAttributionInvalidate: {
		TargetArgument: true,
	},
	ActionPositionInvalidate: {
		TargetArena: true, TargetArgument: true,
	},
	ActionSuspension: {
		TargetProfile: true,
	},
	ActionBan: {
		TargetProfile: true,
	},
	ActionPreserveLegal: {
		TargetArena: true, TargetArgument: true, TargetProfile: true,
	},
}

// SanctionAllowed reports whether the action may sanction the target type.
// Unknown actions and unknown targets deny.
func SanctionAllowed(target TargetType, action Action) bool {
	targets, ok := sanctionTargets[action]
	if !ok {
		return false
	}
	return targets[target]
}

// MutatesProjection reports whether applying the action changes a public
// projection in the same transaction as the audit event. Record-only
// actions still write the immutable moderation_actions row: the absence of
// a projection effect is explicit, never a silent skip.
func (a Action) MutatesProjection() bool {
	switch a {
	case ActionArenaClose, ActionArgumentRemove, ActionAttributionInvalidate,
		ActionSuspension, ActionBan:
		return true
	default:
		return false
	}
}
