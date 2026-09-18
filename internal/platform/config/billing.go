package config

import (
	"fmt"
	"net/url"
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

	// billingSuccessURLVariable and billingCancelURLVariable are the
	// allowlisted return URLs of the checkout (P12-T04).
	billingSuccessURLVariable = "ARENA_BILLING_SUCCESS_URL"
	billingCancelURLVariable  = "ARENA_BILLING_CANCEL_URL"

	// maxBillingPriceIDLength bounds a configured provider identifier.
	maxBillingPriceIDLength = 200

	// maxBillingReturnURLLength bounds a configured return URL: it travels to
	// the payment provider, which documents a bounded length for it.
	maxBillingReturnURLLength = 2000
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

// parseBillingReturnURL validates one allowlisted return URL: an absolute
// HTTP(S) URL with a host and without embedded credentials, bounded in length.
// HTTPS is required in production (an external buyer must never be sent back
// over plain HTTP), while development may use a loopback address.
func parseBillingReturnURL(variable, raw string, production bool) (string, ValidationErrors) {
	problem := ""
	parsed, err := url.Parse(raw)
	switch {
	case raw == "":
		problem = "cannot be empty (remove the variable instead of leaving it blank)"
	case err != nil:
		problem = "is not a valid URL"
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		problem = fmt.Sprintf("invalid value %q (want an absolute HTTP(S) URL)", raw)
	case parsed.Host == "":
		problem = fmt.Sprintf("invalid value %q (want a URL with a host, for example https://arena.example/checkout/success)", raw)
	case parsed.User != nil:
		problem = "must not carry credentials"
	case parsed.Fragment != "":
		problem = "must not carry a fragment (the provider appends its own parameters)"
	case len(raw) > maxBillingReturnURLLength:
		problem = fmt.Sprintf("is longer than %d characters", maxBillingReturnURLLength)
	case production && parsed.Scheme != "https":
		problem = "must use HTTPS when ARENA_ENV=production"
	}
	if problem != "" {
		return "", ValidationErrors{{Variable: variable, Problem: problem}}
	}
	return raw, nil
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

// BillingSuccessURL returns the allowlisted URL the provider sends the buyer
// to after paying, empty when this deployment does not serve checkout.
func (config Config) BillingSuccessURL() string { return config.billingSuccessURL }

// BillingCancelURL returns the allowlisted URL the provider sends the buyer to
// after cancelling, empty when this deployment does not serve checkout.
func (config Config) BillingCancelURL() string { return config.billingCancelURL }
