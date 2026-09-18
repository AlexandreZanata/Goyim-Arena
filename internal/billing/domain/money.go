package domain

import (
	"strconv"
	"strings"
)

// Currency is an ISO 4217 currency code supported by the price list. The
// vocabulary is closed: a currency enters the catalog only when a commercial
// region is documented to charge it (docs/MONETIZATION.md §4).
type Currency string

const (
	// CurrencyBRL is the Brazilian real, charged in the BR market.
	CurrencyBRL Currency = "BRL"

	// CurrencyUSD is the US dollar, charged in the INTERNATIONAL market.
	CurrencyUSD Currency = "USD"
)

// AllCurrencies returns the closed vocabulary in canonical order.
func AllCurrencies() []Currency {
	return []Currency{CurrencyBRL, CurrencyUSD}
}

// ParseCurrency canonicalizes a configured or persisted ISO 4217 code.
// Surrounding spaces are ignored and the code is upper-cased; an unsupported
// currency is refused instead of being stored.
func ParseCurrency(raw string) (Currency, error) {
	currency := Currency(strings.ToUpper(strings.TrimSpace(raw)))
	if !currency.IsValid() {
		return "", ErrUnsupportedCurrency
	}
	return currency, nil
}

// IsValid reports whether the currency belongs to the closed vocabulary.
func (c Currency) IsValid() bool {
	switch c {
	case CurrencyBRL, CurrencyUSD:
		return true
	default:
		return false
	}
}

// String returns the canonical ISO 4217 code.
func (c Currency) String() string {
	return string(c)
}

// Money is an exact monetary amount in the minor units of a currency
// (AGENTS.md: monetary values are integers in minor units with an ISO
// currency code, never float). Formatting for a human happens only in the
// presentation layer; the backend transports minor units as they are
// (I18N_STANDARD.md §5).
type Money struct {
	minorUnits int64
	currency   Currency
}

// NewMoney builds an amount in minor units. Negative amounts cannot be
// catalogued and unsupported currencies are refused, so a price can never
// carry an ambiguous unit.
func NewMoney(minorUnits int64, currency Currency) (Money, error) {
	if !currency.IsValid() {
		return Money{}, ErrUnsupportedCurrency
	}
	if minorUnits < 0 {
		return Money{}, ErrInvalidMoney
	}
	return Money{minorUnits: minorUnits, currency: currency}, nil
}

// MinorUnits returns the exact integer amount, e.g. 990 for R$ 9,90.
func (m Money) MinorUnits() int64 {
	return m.minorUnits
}

// Currency returns the currency of the amount.
func (m Money) Currency() Currency {
	return m.currency
}

// IsZero reports whether the amount is zero or uninitialized.
func (m Money) IsZero() bool {
	return m.minorUnits == 0
}

// Equals reports whether two amounts are the same value in the same currency.
func (m Money) Equals(other Money) bool {
	return m.minorUnits == other.minorUnits && m.currency == other.currency
}

// String renders the diagnostic form "BRL 990": currency code and minor units.
// It is a debugging rendering and never a locale-formatted price, because the
// backend does not pre-format money for the API (I18N_STANDARD.md §5).
func (m Money) String() string {
	return m.currency.String() + " " + strconv.FormatInt(m.minorUnits, 10)
}
