package config

import (
	"strings"
	"testing"
)

// TestLoadWithoutBillingConfigurationSellsNothing covers the safe default:
// without ARENA_BILLING_* variables nothing is sellable, and the server still
// boots (the decision to require prices in production belongs to the billing
// module, which owns the versioned catalog).
func TestLoadWithoutBillingConfigurationSellsNothing(t *testing.T) {
	t.Parallel()

	config, err := Load(environ())
	if err != nil {
		t.Fatalf("load without billing variables: %v", err)
	}
	if len(config.BillingMarkets()) != 0 || len(config.BillingPrices()) != 0 {
		t.Fatalf("default billing configuration must be empty, got %v / %v",
			config.BillingMarkets(), config.BillingPrices())
	}

	production, err := Load(environ(
		"ARENA_ENV=production",
		"ARENA_DATABASE_URL=postgres://arena:secret@db.internal:5432/arena",
		"ARENA_STRIPE_SECRET_KEY=sk_live_production",
	))
	if err != nil {
		t.Fatalf("production without billing variables must load: %v", err)
	}
	if len(production.BillingMarkets()) != 0 {
		t.Fatal("billing default must not invent a commercial region")
	}
}

func TestLoadReadsBillingMarketsAndPrices(t *testing.T) {
	t.Parallel()

	config, err := Load(environ(
		"ARENA_BILLING_MARKETS=BR:BRL, INTERNATIONAL:USD ",
		"ARENA_BILLING_PRICE_IDS=BR/ink_10000=price_brazil, INTERNATIONAL/pass_1=price_intl",
	))
	if err != nil {
		t.Fatalf("load valid billing configuration: %v", err)
	}

	markets := config.BillingMarkets()
	if len(markets) != 2 {
		t.Fatalf("BillingMarkets() = %v, want 2 entries", markets)
	}
	if markets[0] != (BillingMarket{Market: "BR", Currency: "BRL"}) {
		t.Errorf("markets[0] = %+v", markets[0])
	}
	if markets[1] != (BillingMarket{Market: "INTERNATIONAL", Currency: "USD"}) {
		t.Errorf("markets[1] = %+v", markets[1])
	}

	prices := config.BillingPrices()
	if len(prices) != 2 {
		t.Fatalf("BillingPrices() = %v, want 2 entries", prices)
	}
	if prices[0] != (BillingPrice{Market: "BR", Product: "ink_10000", PriceID: "price_brazil"}) {
		t.Errorf("prices[0] = %+v", prices[0])
	}
	if prices[1] != (BillingPrice{Market: "INTERNATIONAL", Product: "pass_1", PriceID: "price_intl"}) {
		t.Errorf("prices[1] = %+v", prices[1])
	}
}

// TestBillingConfigurationIsNotCanonicalizedHere documents the split: config
// validates the syntax of the variables, while the vocabulary and the catalog
// coherence belong to the billing module, which canonicalizes the values.
func TestBillingConfigurationIsNotCanonicalizedHere(t *testing.T) {
	t.Parallel()

	config, err := Load(environ(
		"ARENA_BILLING_MARKETS=br:brl",
		"ARENA_BILLING_PRICE_IDS=br/ink_10000=price_brazil",
	))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if markets := config.BillingMarkets(); len(markets) != 1 || markets[0].Market != "br" || markets[0].Currency != "brl" {
		t.Errorf("BillingMarkets() = %v, want the operator value trimmed but unchanged", markets)
	}
}

func TestLoadAccumulatesBillingProblems(t *testing.T) {
	t.Parallel()

	_, err := Load(environ(
		"ARENA_BILLING_MARKETS=BR,"+ // no currency
			":BRL,"+ // no market
			"BR:,"+ // no currency value
			"BR:BRL:USD,"+ // too many separators
			"BR:BRL,"+ // valid
			"br:brl", // duplicate of the valid one
		"ARENA_BILLING_PRICE_IDS=BR/ink_10000,"+ // missing the price
			"ink_10000=price_brazil,"+ // missing the market
			"/ink_10000=price_brazil,"+ // empty market
			"BR/=price_brazil,"+ // empty product
			"BR/ink_10000=,"+ // empty price
			"BR/ink_10000/a=price_brazil,"+ // too many separators
			"BR/ink_10000=price=brazil,"+ // too many separators
			"BR/ink_10000=price_brazil,"+ // valid
			"br/ink_10000=price_other,"+ // duplicate of the valid one
			"BR/pass_1=price with space", // whitespace in the identifier
	))
	if err == nil {
		t.Fatal("expected accumulated billing validation errors")
	}

	for _, variable := range []string{"ARENA_BILLING_MARKETS", "ARENA_BILLING_PRICE_IDS"} {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("error should mention %s: %v", variable, err)
		}
	}
	for _, problem := range []string{
		"want MARKET:CURRENCY",
		"duplicate entry for market",
		"want MARKET/PRODUCT=PRICE_ID",
		"duplicate entry for br/ink_10000",
		"contains whitespace",
	} {
		if !strings.Contains(err.Error(), problem) {
			t.Errorf("error should explain %q: %v", problem, err)
		}
	}
}

func TestBillingConfigurationIsImmutableThroughItsGetters(t *testing.T) {
	t.Parallel()

	config, err := Load(environ(
		"ARENA_BILLING_MARKETS=BR:BRL",
		"ARENA_BILLING_PRICE_IDS=BR/ink_10000=price_brazil",
	))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	markets := config.BillingMarkets()
	markets[0] = BillingMarket{Market: "MUTATED", Currency: "MUTATED"}
	if config.BillingMarkets()[0].Market != "BR" {
		t.Error("Config must not expose its internal market slice")
	}

	prices := config.BillingPrices()
	prices[0] = BillingPrice{Market: "MUTATED", Product: "MUTATED", PriceID: "MUTATED"}
	if config.BillingPrices()[0].Product != "ink_10000" {
		t.Error("Config must not expose its internal price slice")
	}
}
