package domain

import (
	"sort"
	"strings"
)

// ProductID is the stable, human-readable identifier of a catalog product.
//
// It is a public key: it names a product in configuration, in the ledger
// reference of a grant and in support conversations. Identifiers are lower
// snake case so they survive environment variables, URLs and logs unchanged.
type ProductID string

// maxProductIDLength bounds the identifier so it stays readable in config and
// ledger references.
const maxProductIDLength = 64

// ParseProductID validates a configured or persisted product identifier.
// Lower-case letters, digits and underscores are accepted; the identifier must
// start with a letter and be at least three characters long.
func ParseProductID(raw string) (ProductID, error) {
	id := ProductID(raw)
	if len(id) < 3 || len(id) > maxProductIDLength {
		return "", ErrInvalidProductID
	}
	for index := 0; index < len(id); index++ {
		character := id[index]
		switch {
		case character >= 'a' && character <= 'z':
		case character == '_' && index > 0:
		case character >= '0' && character <= '9' && index > 0:
		default:
			return "", ErrInvalidProductID
		}
	}
	if id[len(id)-1] == '_' {
		return "", ErrInvalidProductID
	}
	return id, nil
}

// String returns the stored identifier.
func (id ProductID) String() string {
	return string(id)
}

// StripePriceID is the Stripe price object that a product is sold through.
//
// It is environment specific: development and test may leave it unset until
// the price is provisioned, and production requires it (P12-T01). The value
// is an opaque provider identifier, so it is validated structurally and never
// interpreted here.
type StripePriceID string

// maxStripePriceIDLength bounds the provider identifier defensively.
const maxStripePriceIDLength = 200

// ParseStripePriceID validates a configured price identifier. The empty value
// is valid and means "not provisioned in this environment"; anything else must
// carry the Stripe price prefix.
func ParseStripePriceID(raw string) (StripePriceID, error) {
	id := StripePriceID(raw)
	if id.IsZero() {
		return "", nil
	}
	if len(id) > maxStripePriceIDLength || !strings.HasPrefix(string(id), "price_") {
		return "", ErrInvalidStripePriceID
	}
	remainder := strings.TrimPrefix(string(id), "price_")
	if remainder == "" {
		return "", ErrInvalidStripePriceID
	}
	for index := 0; index < len(remainder); index++ {
		character := remainder[index]
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		default:
			return "", ErrInvalidStripePriceID
		}
	}
	return id, nil
}

// IsSet reports whether a price has been provisioned for this environment.
func (id StripePriceID) IsSet() bool {
	return id != ""
}

// IsZero reports whether the price is unset.
func (id StripePriceID) IsZero() bool {
	return id == ""
}

// String returns the stored identifier.
func (id StripePriceID) String() string {
	return string(id)
}

// Product is one sellable catalog entry: a product of a commercial region,
// its exact price in that region's currency, the entitlement it confers and
// the Stripe price it is sold through.
type Product struct {
	market  Market
	id      ProductID
	amount  Money
	grant   Grant
	priceID StripePriceID
}

// NewProduct builds a catalog entry. The price must be expressed in the
// currency the market charges, so a product can never be sold in the wrong
// unit, and an unset Stripe price is allowed because price identifiers are
// environment specific.
func NewProduct(market Market, id ProductID, amount Money, grant Grant, priceID StripePriceID) (Product, error) {
	product := Product{market: market, id: id, amount: amount, grant: grant, priceID: priceID}
	if err := product.Validate(); err != nil {
		return Product{}, err
	}
	return product, nil
}

// WithPriceID returns a copy of the product sold through the given Stripe
// price; the original entry is never mutated.
func (p Product) WithPriceID(priceID StripePriceID) (Product, error) {
	priced := p
	priced.priceID = priceID
	if err := priced.Validate(); err != nil {
		return Product{}, err
	}
	return priced, nil
}

// Market returns the commercial region of the product.
func (p Product) Market() Market {
	return p.market
}

// ID returns the stable product identifier.
func (p Product) ID() ProductID {
	return p.id
}

// Amount returns the exact price in minor units.
func (p Product) Amount() Money {
	return p.amount
}

// Currency returns the currency of the price.
func (p Product) Currency() Currency {
	return p.amount.Currency()
}

// Grant returns the entitlement the product confers.
func (p Product) Grant() Grant {
	return p.grant
}

// PriceID returns the Stripe price, empty when not provisioned.
func (p Product) PriceID() StripePriceID {
	return p.priceID
}

// IsPriced reports whether the product has a Stripe price in this environment.
func (p Product) IsPriced() bool {
	return p.priceID.IsSet()
}

// Equals reports whether two products are exactly the same entry.
func (p Product) Equals(other Product) bool {
	return p.market == other.market &&
		p.id == other.id &&
		p.amount.Equals(other.amount) &&
		p.grant.Equals(other.grant) &&
		p.priceID == other.priceID
}

// String renders the diagnostic form "BR/ink_10000".
func (p Product) String() string {
	return p.market.String() + "/" + p.id.String()
}

// Validate enforces the catalog invariants of one entry: a known market and
// product identifier, a positive price in the currency the market charges and
// a grant with exactly the quantities its kind describes.
func (p Product) Validate() error {
	if !p.market.IsValid() {
		return ErrInvalidMarket
	}
	if _, err := ParseProductID(string(p.id)); err != nil {
		return err
	}
	if !p.amount.Currency().IsValid() {
		return ErrUnsupportedCurrency
	}
	if p.amount.MinorUnits() < 1 {
		return ErrInvalidMoney
	}
	expected, err := p.market.Currency()
	if err != nil {
		return err
	}
	if p.amount.Currency() != expected {
		return ErrMarketCurrencyMismatch
	}
	if err := p.grant.Validate(); err != nil {
		return err
	}
	if _, err := ParseStripePriceID(string(p.priceID)); err != nil {
		return err
	}
	return nil
}

// Catalog is the versioned, validated price list of the deployment.
//
// A catalog is immutable: changing a price means publishing a new version, so
// rights already acquired are never rewritten retroactively
// (docs/MONETIZATION.md §4). The version accompanies the catalog so a
// purchase, a grant and a support conversation can always name the price list
// that was in force.
type Catalog struct {
	version  int
	products []Product
}

// NewCatalog validates the version and the entries and stores them in a
// canonical order: by market, then by product identifier. Duplicate entries of
// the same market and product are refused, because a price list with two
// prices for one product cannot be resolved deterministically.
func NewCatalog(version int, products []Product) (*Catalog, error) {
	if version < 1 {
		return nil, ErrInvalidCatalogVersion
	}

	ordered := make([]Product, len(products))
	copy(ordered, products)
	for _, product := range ordered {
		if err := product.Validate(); err != nil {
			return nil, err
		}
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].market != ordered[right].market {
			return ordered[left].market < ordered[right].market
		}
		return ordered[left].id < ordered[right].id
	})
	for index := 1; index < len(ordered); index++ {
		if ordered[index-1].market == ordered[index].market && ordered[index-1].id == ordered[index].id {
			return nil, ErrDuplicateProduct
		}
	}

	return &Catalog{version: version, products: ordered}, nil
}

// Version returns the catalog version.
func (c *Catalog) Version() int {
	return c.version
}

// Products returns every entry in canonical order. The slice is a copy, so a
// caller cannot mutate the catalog.
func (c *Catalog) Products() []Product {
	products := make([]Product, len(c.products))
	copy(products, c.products)
	return products
}

// Markets returns the distinct markets of the catalog in canonical order.
func (c *Catalog) Markets() []Market {
	markets := make([]Market, 0, len(AllMarkets()))
	for _, market := range AllMarkets() {
		for _, product := range c.products {
			if product.market == market {
				markets = append(markets, market)
				break
			}
		}
	}
	return markets
}

// ProductsForMarket returns the entries of one market in canonical order.
func (c *Catalog) ProductsForMarket(market Market) []Product {
	products := make([]Product, 0, len(c.products))
	for _, product := range c.products {
		if product.market == market {
			products = append(products, product)
		}
	}
	return products
}

// Has reports whether the catalog carries an entry for the market and product,
// regardless of the entry being priced in this environment.
func (c *Catalog) Has(market Market, id ProductID) bool {
	_, found := c.find(market, id)
	return found
}

// Product resolves one sellable entry. A product that is absent from the
// catalog is unknown; a product whose Stripe price is not provisioned in this
// environment is refused separately, so a caller can never charge an amount it
// cannot settle.
func (c *Catalog) Product(market Market, id ProductID) (Product, error) {
	product, found := c.find(market, id)
	if !found {
		return Product{}, ErrUnknownProduct
	}
	if !product.IsPriced() {
		return Product{}, ErrProductNotPriced
	}
	return product, nil
}

// find locates one entry without applying the priced requirement.
func (c *Catalog) find(market Market, id ProductID) (Product, bool) {
	for _, product := range c.products {
		if product.market == market && product.id == id {
			return product, true
		}
	}
	return Product{}, false
}
