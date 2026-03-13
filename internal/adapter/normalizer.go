// internal/adapter/normalizer.go
package adapter

import "strings"

// SymbolPair represents a normalized trading pair with base and quote assets.
type SymbolPair struct {
	Base  string
	Quote string
}

// knownQuotes lists common quote currencies used for splitting concatenated
// symbols (e.g., "BTCUSDT"). Longer quotes are checked first so "BUSD" is
// matched before "USD".
var knownQuotes = []string{
	"USDT", "BUSD", "USDC", "TUSD",
	"USD", "BTC", "ETH", "BNB",
	"EUR", "GBP", "DAI",
}

// ParseSymbol parses any common symbol format into a SymbolPair.
// Supported formats:
//   - "BTC-USDT"  (Bybit / canonical)
//   - "BTC/USDT"  (generic / CCXT)
//   - "BTCUSDT"   (Binance)
//   - "btc-usdt"  (case-insensitive)
func ParseSymbol(raw string) SymbolPair {
	raw = strings.TrimSpace(raw)
	upper := strings.ToUpper(raw)

	// Try splitting on common delimiters first.
	for _, sep := range []string{"-", "/", "_"} {
		if parts := strings.SplitN(upper, sep, 2); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return SymbolPair{Base: parts[0], Quote: parts[1]}
		}
	}

	// No delimiter found – try matching known quote currencies.
	for _, q := range knownQuotes {
		if strings.HasSuffix(upper, q) {
			base := upper[:len(upper)-len(q)]
			if base != "" {
				return SymbolPair{Base: base, Quote: q}
			}
		}
	}

	// Fallback: treat the whole string as the base with an empty quote.
	return SymbolPair{Base: upper, Quote: ""}
}

// String returns the canonical internal format: "BASE-QUOTE".
func (sp SymbolPair) String() string {
	return sp.Base + "-" + sp.Quote
}

// ForBinance returns the Binance format: "BTCUSDT" (concatenated, no delimiter).
func (sp SymbolPair) ForBinance() string {
	return sp.Base + sp.Quote
}

// ForBybit returns the Bybit format: "BTC-USDT".
func (sp SymbolPair) ForBybit() string {
	return sp.Base + "-" + sp.Quote
}

// ForOKX returns the OKX format: "BTC-USDT".
func (sp SymbolPair) ForOKX() string {
	return sp.Base + "-" + sp.Quote
}

// ForGeneric returns the generic/CCXT format: "BTC/USDT".
func (sp SymbolPair) ForGeneric() string {
	return sp.Base + "/" + sp.Quote
}
