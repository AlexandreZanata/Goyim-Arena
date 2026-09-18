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

// TestBillingReturnURLsAreAllowlisted covers the checkout return URLs
// (P12-T04): they are configuration, they must share one origin, and they must
// be HTTPS in production — the checkout can never become an open redirect.
func TestBillingReturnURLsAreAllowlisted(t *testing.T) {
	t.Parallel()

	config, err := Load(environ(
		"ARENA_BILLING_SUCCESS_URL=https://arena.example/checkout/success?product=ink",
		"ARENA_BILLING_CANCEL_URL=https://arena.example/checkout/cancel",
	))
	if err != nil {
		t.Fatalf("load valid return URLs: %v", err)
	}
	if config.BillingSuccessURL() != "https://arena.example/checkout/success?product=ink" {
		t.Errorf("success URL = %q", config.BillingSuccessURL())
	}
	if config.BillingCancelURL() != "https://arena.example/checkout/cancel" {
		t.Errorf("cancel URL = %q", config.BillingCancelURL())
	}

	unset, err := Load(environ())
	if err != nil {
		t.Fatalf("return URLs are optional in the environment: %v", err)
	}
	if unset.BillingSuccessURL() != "" || unset.BillingCancelURL() != "" {
		t.Error("no return URL must be invented by default")
	}

	valid := []struct {
		name     string
		success  string
		cancel   string
		envValue string
	}{
		{name: "loopback in development", success: "http://127.0.0.1:8080/checkout/success", cancel: "http://127.0.0.1:8080/checkout/cancel"},
		{name: "same origin with a port", success: "https://arena.example:8443/ok", cancel: "https://arena.example:8443/no"},
		{name: "https in production", success: "https://arena.example/ok", cancel: "https://arena.example/no", envValue: "production"},
	}
	for _, testCase := range valid {
		t.Run("valid/"+testCase.name, func(t *testing.T) {
			t.Parallel()
			environment := environ(
				"ARENA_BILLING_SUCCESS_URL="+testCase.success,
				"ARENA_BILLING_CANCEL_URL="+testCase.cancel,
			)
			if testCase.envValue == "production" {
				environment = environ(
					"ARENA_ENV=production",
					"ARENA_DATABASE_URL=postgres://arena:secret@db.internal:5432/arena",
					"ARENA_STRIPE_SECRET_KEY=sk_live_urls",
					"ARENA_BILLING_SUCCESS_URL="+testCase.success,
					"ARENA_BILLING_CANCEL_URL="+testCase.cancel,
				)
			}
			if _, err := Load(environment); err != nil {
				t.Fatalf("load: %v", err)
			}
		})
	}

	invalid := []struct {
		name    string
		success string
		cancel  string
		want    string
	}{
		{name: "relative success URL", success: "/checkout/success", cancel: "https://arena.example/no", want: "absolute HTTP(S)"},
		{name: "javascript scheme", success: "javascript:alert(1)", cancel: "https://arena.example/no", want: "absolute HTTP(S)"},
		{name: "missing host", success: "https:///success", cancel: "https://arena.example/no", want: "host"},
		{name: "embedded credentials", success: "https://user:pass@arena.example/ok", cancel: "https://arena.example/no", want: "credentials"},
		{name: "fragment", success: "https://arena.example/ok#fragment", cancel: "https://arena.example/no", want: "fragment"},
		{name: "empty value", success: "", cancel: "https://arena.example/no", want: "cannot be empty"},
		{name: "different origins", success: "https://arena.example/ok", cancel: "https://evil.example/no", want: "share the origin"},
		{name: "different schemes", success: "https://arena.example/ok", cancel: "http://arena.example/no", want: "share the origin"},
		{name: "different ports", success: "https://arena.example/ok", cancel: "https://arena.example:8443/no", want: "share the origin"},
	}
	for _, testCase := range invalid {
		t.Run("invalid/"+testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(environ(
				"ARENA_BILLING_SUCCESS_URL="+testCase.success,
				"ARENA_BILLING_CANCEL_URL="+testCase.cancel,
			))
			if err == nil {
				t.Fatalf("return URLs %q / %q must be refused", testCase.success, testCase.cancel)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("error should explain %q: %v", testCase.want, err)
			}
			if !strings.Contains(err.Error(), "ARENA_BILLING_") {
				t.Errorf("error must name the variable: %v", err)
			}
		})
	}

	t.Run("plain HTTP is refused in production", func(t *testing.T) {
		t.Parallel()
		_, err := Load(environ(
			"ARENA_ENV=production",
			"ARENA_DATABASE_URL=postgres://arena:secret@db.internal:5432/arena",
			"ARENA_STRIPE_SECRET_KEY=sk_live_urls",
			"ARENA_BILLING_SUCCESS_URL=http://arena.example/ok",
			"ARENA_BILLING_CANCEL_URL=http://arena.example/no",
		))
		if err == nil {
			t.Fatal("plain HTTP return URLs must be refused in production")
		}
		if !strings.Contains(err.Error(), "HTTPS") {
			t.Errorf("error should name the production rule: %v", err)
		}
	})
}
