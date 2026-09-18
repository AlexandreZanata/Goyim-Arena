package config

import (
	"fmt"
	"strings"
)

// Billing configuration (P12-T01).
//
// The deployment declares which commercial regions are enabled, the currency
// each one charges and the Stripe price ID of every catalog product:
//
//	ARENA_BILLING_MARKETS=BR:BRL,INTERNATIONAL:USD
//	ARENA_BILLING_PRICE_IDS=BR/ink_10000=price_...,INTERNATIONAL/pass_1=price_...
//
// Amounts are deliberately NOT read from the environment: they come from the
// versioned catalog shipped with the binary
// (internal/billing/adapters/catalog/products.json), so a price can only change
// through a reviewed catalog version and the Stripe price identifiers stay the
// only environment-specific billing values.
//
// This file validates the syntax of those two variables (shape, duplicates)
// and accumulates every problem keyed by variable name, like the rest of
// config. Vocabulary and catalog coherence (known region, known product,
// currency of the region, production requiring a price) are validated by the
// billing module when the catalog is built, because they need the versioned
// catalog that config does not own.
const (
	// billingMarketsVariable lists the enabled commercial regions.
	billingMarketsVariable = "ARENA_BILLING_MARKETS"

	// billingPriceIDsVariable lists the Stripe price of each product.
	billingPriceIDsVariable = "ARENA_BILLING_PRICE_IDS"

	// maxBillingPriceIDLength bounds a configured provider identifier.
	maxBillingPriceIDLength = 200
)

// BillingMarket is one enabled commercial region and the currency it charges.
type BillingMarket struct {
	Market   string
	Currency string
}

// BillingPrice is the Stripe price configured for one catalog product.
type BillingPrice struct {
	Market  string
	Product string
	PriceID string
}

// parseBillingMarkets validates the ARENA_BILLING_MARKETS syntax: a
// comma-separated list of MARKET:CURRENCY entries. Values are only trimmed
// here; canonicalization and vocabulary checks belong to the billing module.
func parseBillingMarkets(raw string) ([]BillingMarket, ValidationErrors) {
	var validationErrors ValidationErrors
	var markets []BillingMarket
	seen := make(map[string]bool)

	for _, entry := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			validationErrors = append(validationErrors, ValidationError{
				Variable: billingMarketsVariable,
				Problem:  "contains an empty entry (want a list like BR:BRL,INTERNATIONAL:USD)",
			})
			continue
		}
		market, currency, found := strings.Cut(trimmed, ":")
		if !found || market == "" || currency == "" || strings.Contains(currency, ":") {
			validationErrors = append(validationErrors, ValidationError{
				Variable: billingMarketsVariable,
				Problem:  fmt.Sprintf("invalid entry %q (want MARKET:CURRENCY, for example BR:BRL)", entry),
			})
			continue
		}
		key := strings.ToUpper(market)
		if seen[key] {
			validationErrors = append(validationErrors, ValidationError{
				Variable: billingMarketsVariable,
				Problem:  fmt.Sprintf("duplicate entry for market %q", market),
			})
			continue
		}
		seen[key] = true
		markets = append(markets, BillingMarket{Market: market, Currency: currency})
	}

	return markets, validationErrors
}

// parseBillingPrices validates the ARENA_BILLING_PRICE_IDS syntax: a
// comma-separated list of MARKET/PRODUCT=PRICE_ID entries. Values are only
// trimmed here; the Stripe identifier shape and the product vocabulary belong
// to the billing module.
func parseBillingPrices(raw string) ([]BillingPrice, ValidationErrors) {
	var validationErrors ValidationErrors
	var prices []BillingPrice
	seen := make(map[string]bool)

	for _, entry := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			validationErrors = append(validationErrors, ValidationError{
				Variable: billingPriceIDsVariable,
				Problem:  "contains an empty entry (want a list like BR/ink_10000=price_...)",
			})
			continue
		}
		target, priceID, found := strings.Cut(trimmed, "=")
		if !found {
			validationErrors = append(validationErrors, ValidationError{
				Variable: billingPriceIDsVariable,
				Problem:  fmt.Sprintf("invalid entry %q (want MARKET/PRODUCT=PRICE_ID, for example BR/ink_10000=price_...)", entry),
			})
			continue
		}
		market, product, found := strings.Cut(target, "/")
		problem := ""
		switch {
		case !found || market == "" || product == "":
			problem = fmt.Sprintf("invalid entry %q (want MARKET/PRODUCT=PRICE_ID, for example BR/ink_10000=price_...)", entry)
		case strings.Contains(product, "/") || strings.Contains(priceID, "="):
			problem = fmt.Sprintf("invalid entry %q (contains too many separators)", entry)
		case priceID == "":
			problem = fmt.Sprintf("invalid entry %q (missing the Stripe price identifier)", entry)
		case len(priceID) > maxBillingPriceIDLength:
			problem = fmt.Sprintf("invalid entry %q (Stripe price identifier is longer than %d characters)", entry, maxBillingPriceIDLength)
		case strings.ContainsAny(priceID, " \t\r\n"):
			problem = fmt.Sprintf("invalid entry %q (Stripe price identifier contains whitespace)", entry)
		}
		if problem != "" {
			validationErrors = append(validationErrors, ValidationError{
				Variable: billingPriceIDsVariable,
				Problem:  problem,
			})
			continue
		}
		key := strings.ToUpper(market) + "/" + strings.ToUpper(product)
		if seen[key] {
			validationErrors = append(validationErrors, ValidationError{
				Variable: billingPriceIDsVariable,
				Problem:  fmt.Sprintf("duplicate entry for %s/%s", market, product),
			})
			continue
		}
		seen[key] = true
		prices = append(prices, BillingPrice{Market: market, Product: product, PriceID: priceID})
	}

	return prices, validationErrors
}

// BillingMarkets returns the enabled commercial regions of this deployment,
// empty when none is configured. The slice is a copy, so the immutable Config
// cannot be mutated through it.
func (config Config) BillingMarkets() []BillingMarket {
	markets := make([]BillingMarket, len(config.billingMarkets))
	copy(markets, config.billingMarkets)
	return markets
}

// BillingPrices returns the configured Stripe prices of this deployment, empty
// when none is configured. The slice is a copy, so the immutable Config cannot
// be mutated through it.
func (config Config) BillingPrices() []BillingPrice {
	prices := make([]BillingPrice, len(config.billingPrices))
	copy(prices, config.billingPrices)
	return prices
}
