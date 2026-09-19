// Package ratelimit bounds how often one actor may repeat one action
// (P16-T03).
//
// The layer exists because a request bound is not an abuse bound. httplimits
// says how much one request may cost; it says nothing about a client sending
// the same cheap request ten thousand times, and every expensive thing in this
// product is cheap to ask for and costly to serve: creating an account costs an
// Argon2id hash and an email, requesting a password reset sends mail, filing a
// report spends a moderator's attention, opening a checkout calls Stripe.
//
// The design is a policy table plus one bounded mechanism:
//
//   - an Action names the operation, and the table names its budget. The
//     table is the policy, in one place, with the reasoning next to each row,
//     so a budget is reviewed rather than discovered inside a handler;
//   - a Budget is a token bucket: Burst requests back to back, with the
//     full burst refilled over Window. Burst covers a person clicking and
//     retrying; the window covers the machine;
//   - every action is bounded by the caller's network address, and an
//     authenticated action also by the account. The two dimensions catch two
//     different attacks: one machine cycling accounts, and many machines
//     against one account. The address budget is deliberately looser than the
//     account budget, because an address is an imperfect signal (carrier NAT
//     puts thousands of people behind one address) while an account is exact;
//   - the mechanism is bounded in memory: at most a configured number of keys
//     are held, the least recently used are evicted, and idle keys expire. It
//     is a single-process, in-memory limiter, which is a documented limit
//     rather than a hidden one (docs/SECURITY.md section 6): with N instances
//     the effective budget is N times the table, and closing that gap needs a
//     shared store, which the MVP does not have.
//
// What this package deliberately does not do: it never treats an address as
// proof of abuse, it never persists a key, it never logs one, and a decision
// never carries the value it was keyed on.
package ratelimit

import "time"

// Action is one throttled operation of the product. Actions are stable names:
// they appear in the policy table, in tests and in operational reasoning, and
// they are not derived from the request path, so a route cannot silently
// acquire a different budget by moving.
type Action string

// The declared actions. Each one is an operation that creates an account,
// sends a message, spends money, calls a third party or asks a human to read
// something — the things that are cheap to request and expensive to serve.
const (
	// ActionAuthRegister creates an account: an Argon2id hash, a verification
	// token and an email.
	ActionAuthRegister Action = "auth.register"
	// ActionAuthLogin verifies a password: the endpoint a credential-stuffing
	// run aims at.
	ActionAuthLogin Action = "auth.login"
	// ActionAuthPasswordResetRequest sends a reset email.
	ActionAuthPasswordResetRequest Action = "auth.password_reset_request"
	// ActionAuthPasswordResetConfirm guesses at a reset token.
	ActionAuthPasswordResetConfirm Action = "auth.password_reset_confirm"
	// ActionPositionConfirm records the immutable initial position.
	ActionPositionConfirm Action = "position.confirm"
	// ActionPositionChange records a position change.
	ActionPositionChange Action = "position.change"
	// ActionArgumentPublish creates argument content (a root argument or a
	// reply): the product's only user-generated content.
	ActionArgumentPublish Action = "argument.publish"
	// ActionReportFile files a moderation report, which spends human review
	// capacity rather than machine capacity.
	ActionReportFile Action = "report.file"
	// ActionCheckoutCreate opens a checkout session, which calls Stripe.
	ActionCheckoutCreate Action = "checkout.create"
	// ActionBillingPortal opens the hosted billing portal, which calls Stripe.
	ActionBillingPortal Action = "billing.portal"
	// ActionMFAVerify checks a second factor code (step-up or recovery).
	ActionMFAVerify Action = "mfa.verify"
)

// Budget is one token bucket: Burst requests may be spent back to back, and
// the bucket refills to Burst over Window.
//
// The two numbers are the two questions an operator asks: how many times in a
// row is a person allowed to retry (Burst), and how fast may a machine go
// (Burst/Window).
type Budget struct {
	// Burst is the capacity of the bucket, in requests.
	Burst int
	// Window is how long the bucket takes to refill completely. One request
	// is regained every Window/Burst.
	Window time.Duration
}

// rate is the refill rate in tokens per second.
//
// A non-positive Window means the bucket never refills, which is the
// conservative reading of an incomplete row: the Burst is spent once and the
// budget stays exhausted. A non-positive Burst allows nothing at all, which is
// the same direction.
func (budget Budget) rate() float64 {
	if budget.Burst <= 0 {
		return 0
	}
	if budget.Window <= 0 {
		return 0
	}
	return float64(budget.Burst) / budget.Window.Seconds()
}

// Policy is the budget of one action.
//
// Address is mandatory: no action is unbounded, and a policy that forgot the
// address dimension would be unbounded for an unauthenticated caller. Account
// is optional and applies only when the caller is authenticated.
type Policy struct {
	// Address is the budget of the caller's network address.
	Address Budget
	// Account is the budget of the authenticated account, when there is one.
	Account *Budget
}

// DefaultPolicy is the budget of an action the table does not name.
//
// It exists so that a new throttled action cannot be unbounded by omission.
// It is tight and address-only on purpose: an undeclared action is a
// programming gap, and the safe reading of a gap is the smallest budget, not
// the largest. A test asserts the table covers every declared Action, so the
// default is a safety net rather than a path in use.
var DefaultPolicy = Policy{Address: Budget{Burst: 20, Window: time.Minute}}

// policies is the whole policy, in one readable place. The numbers are
// deliberately conservative starting points, derived from what one accepted
// request costs to serve, and they are expected to be tuned against real
// traffic (phase 28). Tightening a row is a normal review; loosening one is a
// decision that needs a reason.
var policies = map[Action]Policy{
	// Five accounts per hour from one address: enough for a household, far
	// short of a farm. Each accepted request hashes a password and sends mail.
	ActionAuthRegister: {Address: Budget{Burst: 5, Window: time.Hour}},

	// Ten attempts per minute, refilling over the minute: a person mistyping a
	// password never notices, a password-guessing loop is stopped after ten
	// tries and stays limited to ten per minute.
	ActionAuthLogin: {Address: Budget{Burst: 10, Window: time.Minute}},

	// Three reset emails per hour per address: mail is a cost paid by someone
	// else, and a reset request is also an existence oracle.
	ActionAuthPasswordResetRequest: {Address: Budget{Burst: 3, Window: time.Hour}},

	// Ten token attempts per hour: a reset token is high entropy, so this is
	// about noise and hashing cost, not about a realistic guess.
	ActionAuthPasswordResetConfirm: {Address: Budget{Burst: 10, Window: time.Hour}},

	// A participant confirms one initial position per arena, so ten per minute
	// per account is generous; the address budget is loose because a whole
	// carrier NAT browses the same arenas.
	ActionPositionConfirm: {Address: Budget{Burst: 60, Window: time.Minute}, Account: &Budget{Burst: 10, Window: time.Minute}},

	// Position changes are deliberate moves, not streams.
	ActionPositionChange: {Address: Budget{Burst: 60, Window: time.Minute}, Account: &Budget{Burst: 10, Window: time.Minute}},

	// Publishing spends INK, which the ledger bounds by design; the rate limit
	// bounds drafting loops and reply floods, which the ledger would happily
	// charge for.
	ActionArgumentPublish: {Address: Budget{Burst: 60, Window: time.Minute}, Account: &Budget{Burst: 12, Window: time.Minute}},

	// A report spends a moderator's attention. Six per hour per account, and a
	// looser network bound so that a public network cannot silence a whole
	// building's abuse reports.
	ActionReportFile: {Address: Budget{Burst: 60, Window: time.Hour}, Account: &Budget{Burst: 6, Window: time.Hour}},

	// Every accepted checkout calls Stripe. Eight per hour per account is more
	// than a person retrying a declined card needs.
	ActionCheckoutCreate: {Address: Budget{Burst: 40, Window: time.Hour}, Account: &Budget{Burst: 8, Window: time.Hour}},

	// Opening the hosted portal is the same external call with less state.
	ActionBillingPortal: {Address: Budget{Burst: 40, Window: time.Hour}, Account: &Budget{Burst: 8, Window: time.Hour}},

	// A second factor code is a secret with few enough digits that guessing it
	// is a matter of volume: ten attempts per hour per account, with a looser
	// network bound so that a shared address cannot silence the check. The
	// account dimension is the one that matters here, because the caller is
	// already authenticated.
	ActionMFAVerify: {Address: Budget{Burst: 60, Window: time.Hour}, Account: &Budget{Burst: 10, Window: time.Hour}},
}

// Actions lists every declared action, so completeness is testable and so
// operational documentation can enumerate the policies without reading the
// map.
func Actions() []Action {
	actions := make([]Action, 0, len(policies))
	for action := range policies {
		actions = append(actions, action)
	}
	return actions
}

// PolicyFor resolves the budget of an action. The second result reports
// whether the table declares the action; a caller that wants to know whether a
// budget was decided or defaulted can tell the two apart.
func PolicyFor(action Action) (Policy, bool) {
	policy, declared := policies[action]
	if !declared {
		return DefaultPolicy, false
	}
	return policy, true
}
