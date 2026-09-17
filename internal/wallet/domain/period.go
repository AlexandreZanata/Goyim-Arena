package domain

import "time"

// FreeMonthlyFranchise is the monthly FREE_INK franchise of the Free plan
// (docs/MONETIZATION.md §2.2).
const FreeMonthlyFranchise int64 = 5000

// FreeCyclePolicy carries the monthly renewal terms of the free plan.
type FreeCyclePolicy struct {
	Franchise Ink
}

// DefaultFreeCyclePolicy returns the standard free plan terms.
func DefaultFreeCyclePolicy() FreeCyclePolicy {
	return FreeCyclePolicy{Franchise: Ink{amount: FreeMonthlyFranchise}}
}

// MonthlyPeriod is one calendar month of a cycle anchored on an instant
// (normally the account activation). Periods are half-open UTC intervals
// [Start, End); the anchor day is clamped to the last day of shorter months,
// so the cycle never skips or doubles a month. Because every boundary is
// evaluated in UTC, a preference timezone can never shift a renewal.
type MonthlyPeriod struct {
	anchor time.Time
	index  int64
	start  time.Time
	end    time.Time
}

// PeriodFor returns the period of the cycle anchored at anchor that contains
// the instant at. Instants before the anchor map to negative indices,
// representing periods before activation.
func PeriodFor(anchor, at time.Time) MonthlyPeriod {
	anchor = anchor.UTC()
	at = at.UTC()

	index := calendarMonthDiff(anchor, at)
	start := addClampedMonths(anchor, index)
	for at.Before(start) {
		index--
		start = addClampedMonths(anchor, index)
	}
	return MonthlyPeriod{
		anchor: anchor,
		index:  index,
		start:  start,
		end:    addClampedMonths(anchor, index+1),
	}
}

// PeriodAt returns the period with the given index of the cycle.
func PeriodAt(anchor time.Time, index int64) MonthlyPeriod {
	anchor = anchor.UTC()
	return MonthlyPeriod{
		anchor: anchor,
		index:  index,
		start:  addClampedMonths(anchor, index),
		end:    addClampedMonths(anchor, index+1),
	}
}

// Index returns how many months separate the period from the anchor period
// (0 is the activation period).
func (p MonthlyPeriod) Index() int64 {
	return p.index
}

// Start returns the inclusive UTC start instant.
func (p MonthlyPeriod) Start() time.Time {
	return p.start
}

// End returns the exclusive UTC end instant.
func (p MonthlyPeriod) End() time.Time {
	return p.end
}

// Contains reports whether the instant falls inside the period.
func (p MonthlyPeriod) Contains(at time.Time) bool {
	at = at.UTC()
	return !at.Before(p.start) && at.Before(p.end)
}

// Next returns the following period of the same cycle.
func (p MonthlyPeriod) Next() MonthlyPeriod {
	return PeriodAt(p.anchor, p.index+1)
}

// calendarMonthDiff counts whole calendar months between two instants in the
// same UTC calendar, ignoring the day of the month.
func calendarMonthDiff(from, to time.Time) int64 {
	return int64(to.Year()-from.Year())*12 + int64(to.Month()) - int64(from.Month())
}

// addClampedMonths adds a whole number of calendar months to the anchor,
// clamping the day of the month to the last valid day (Jan 31 + 1 month is
// Feb 28/29, never Mar 3).
func addClampedMonths(anchor time.Time, months int64) time.Time {
	zeroBasedMonth := int64(anchor.Month()) - 1 + months
	year := anchor.Year() + int(floorDiv(zeroBasedMonth, 12))
	month := time.Month(floorMod(zeroBasedMonth, 12) + 1)

	day := anchor.Day()
	if last := daysInMonth(year, month); day > last {
		day = last
	}

	return time.Date(year, month, day,
		anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), time.UTC)
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func floorDiv(a, b int64) int64 {
	quotient := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		quotient--
	}
	return quotient
}

func floorMod(a, b int64) int64 {
	remainder := a % b
	if remainder != 0 && (remainder < 0) != (b < 0) {
		remainder += b
	}
	return remainder
}
