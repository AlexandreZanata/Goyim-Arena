package domain

import "strings"

// Market is the commercial region of a purchase (docs/MONETIZATION.md §4).
//
// The region is explicit configuration and is never inferred from the buyer's
// IP address: it selects the price list and therefore the currency charged.
// Each market charges exactly one currency, bound by the versioned catalog;
// a value that disagrees with that binding is a configuration error, never a
// silent conversion.
type Market string

const (
	// MarketBrazil is the Brazilian price list, charged in BRL.
	MarketBrazil Market = "BR"

	// MarketInternational is the international price list, charged in USD.
	MarketInternational Market = "INTERNATIONAL"
)

// marketCurrency is the single source of truth binding every market to the
// currency it charges.
var marketCurrency = map[Market]Currency{
	MarketBrazil:        CurrencyBRL,
	MarketInternational: CurrencyUSD,
}

// AllMarkets returns the closed vocabulary in canonical order.
func AllMarkets() []Market {
	return []Market{MarketBrazil, MarketInternational}
}

// ParseMarket canonicalizes a configured or persisted market value. Surrounding
// spaces are ignored and the value is upper-cased, because operators type this
// value into the environment by hand; anything outside the closed vocabulary is
// refused instead of being reflected back.
func ParseMarket(raw string) (Market, error) {
	market := Market(strings.ToUpper(strings.TrimSpace(raw)))
	if !market.IsValid() {
		return "", ErrInvalidMarket
	}
	return market, nil
}

// IsZero reports whether the market is the uninitialized zero value.
func (m Market) IsZero() bool {
	return m == ""
}

// IsValid reports whether the market belongs to the closed vocabulary.
func (m Market) IsValid() bool {
	_, known := marketCurrency[m]
	return known
}

// Currency returns the currency this market charges.
func (m Market) Currency() (Currency, error) {
	currency, known := marketCurrency[m]
	if !known {
		return "", ErrInvalidMarket
	}
	return currency, nil
}

// String returns the canonical market value.
func (m Market) String() string {
	return string(m)
}
