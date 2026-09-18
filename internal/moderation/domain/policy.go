package domain

import "time"

// Action is the moderation action under authorization. It mirrors the
// CHECK constraint of app.moderation_actions (migration 00022) so a value
// accepted here is a value the database accepts.
type Action string

const (
	ActionNoAction              Action = "no_action"
	ActionWarning               Action = "warning"
	ActionLinkHide              Action = "link_hide"
	ActionInteractionLimit      Action = "interaction_limit"
	ActionArgumentRemove        Action = "argument_remove"
	ActionArenaClose            Action = "arena_close"
	ActionAttributionInvalidate Action = "attribution_invalidate"
	ActionPositionInvalidate    Action = "position_invalidate"
	ActionSuspension            Action = "suspension"
	ActionBan                   Action = "ban"
	ActionPreserveLegal         Action = "preserve_legal"
)

// AllActions returns the closed vocabulary in canonical order.
func AllActions() []Action {
	return []Action{
		ActionNoAction, ActionWarning, ActionLinkHide, ActionInteractionLimit,
		ActionArgumentRemove, ActionArenaClose, ActionAttributionInvalidate,
		ActionPositionInvalidate, ActionSuspension, ActionBan, ActionPreserveLegal,
	}
}

// ParseAction validates a requested action against the exact vocabulary.
func ParseAction(raw string) (Action, error) {
	action := Action(raw)
	if !action.IsValid() {
		return "", ErrInvalidAction
	}
	return action, nil
}

// IsValid reports whether the action is an authorized enum value.
func (a Action) IsValid() bool {
	switch a {
	case ActionNoAction, ActionWarning, ActionLinkHide, ActionInteractionLimit,
		ActionArgumentRemove, ActionArenaClose, ActionAttributionInvalidate,
		ActionPositionInvalidate, ActionSuspension, ActionBan, ActionPreserveLegal:
		return true
	default:
		return false
	}
}

// String returns the stored action value.
func (a Action) String() string {
	return string(a)
}

// IsHighImpact reports whether the action changes account access or
// preserves evidence for legal forwarding. High-impact actions always
// require step-up authentication, regardless of role.
func (a Action) IsHighImpact() bool {
	switch a {
	case ActionSuspension, ActionBan, ActionArenaClose, ActionPreserveLegal:
		return true
	default:
		return false
	}
}

// rolePermissions is the single source of truth for what each role may do.
// Moderators handle reversible content triage; admins decide anything;
// security handles safety and legal preservation. Paid, popular and staff
// accounts obey the same matrix: there is no input for payment state or
// popularity anywhere in this policy.
var rolePermissions = map[Role]map[Action]bool{
	RoleModerator: {
		ActionNoAction: true, ActionWarning: true, ActionLinkHide: true,
		ActionInteractionLimit: true, ActionArgumentRemove: true,
		ActionAttributionInvalidate: true, ActionPositionInvalidate: true,
	},
	RoleAdmin: {
		ActionNoAction: true, ActionWarning: true, ActionLinkHide: true,
		ActionInteractionLimit: true, ActionArgumentRemove: true,
		ActionArenaClose: true, ActionAttributionInvalidate: true,
		ActionPositionInvalidate: true, ActionSuspension: true,
		ActionBan: true, ActionPreserveLegal: true,
	},
	RoleSecurity: {
		ActionNoAction: true, ActionLinkHide: true,
		ActionInteractionLimit: true, ActionSuspension: true,
		ActionPreserveLegal: true,
	},
}

// MayPerform reports whether the role may take the action. Unknown roles
// and unknown actions are denied.
func (r Role) MayPerform(action Action) bool {
	allowed, ok := rolePermissions[r]
	if !ok {
		return false
	}
	return allowed[action]
}

// StepUpWindow is how recent the last full authentication must be for a
// high-impact action. It is a policy constant, not configuration: relaxing
// it weakens every suspension, ban, arena closure and legal preservation
// at once, so any change is a conscious edit here with its tests.
const StepUpWindow = 15 * time.Minute

// StepUpSatisfied reports whether the session age meets the step-up rule
// for the action. Low-impact actions accept any non-negative age;
// high-impact actions require age within StepUpWindow. Negative ages are
// incoherent (clock skew or forgery) and never satisfy.
func StepUpSatisfied(action Action, sessionAge time.Duration) (bool, error) {
	if sessionAge < 0 {
		return false, ErrInvalidSessionAge
	}
	if !action.IsValid() {
		return false, ErrInvalidAction
	}
	if !action.IsHighImpact() {
		return true, nil
	}
	return sessionAge <= StepUpWindow, nil
}

// HasConflict reports whether the actor must declare a conflict of interest
// and pass the case to another reviewer (MODERATION §8): the actor is the
// owner of the targeted account or the reporter who filed the case. The
// check uses identifiers only, never emails, payment state or popularity.
func HasConflict(actor, targetOwner, reporter AccountID) bool {
	if actor.IsZero() || targetOwner.IsZero() {
		return false
	}
	if actor == targetOwner {
		return true
	}
	if !reporter.IsZero() && actor == reporter {
		return true
	}
	return false
}
