package runtime

import "strings"

// Price is the list price of a model in US dollars per million tokens.
type Price struct {
	Input, Output, CacheWrite, CacheRead float64
}

// prices holds the API list price of each model, keyed by model id prefix.
// Update it here when the providers change their prices; a model missing
// from the table counts its tokens but no cost. Cache writes are the 5 minute
// rate (1.25× input) and cache reads 0.1× input unless the provider lists
// another value.
var prices = map[string]Price{
	"claude-fable-5-2":  {8, 40, 10, 0.20},
	"claude-mythos-5-2": {8, 40, 10, 0.20},
	"claude-fable-5-1":  {10, 50, 12.5, 1},
	"claude-mythos-5-1": {10, 50, 12.5, 1},
	"claude-fable-5":    {10, 50, 12.5, 1},
	"claude-opus-5-5":   {4, 20, 5, 0.20},
	"claude-opus-5":     {5, 25, 6.25, 0.5},
	"claude-opus-4-8":   {5, 25, 6.25, 0.5},
	"claude-opus-4-7":   {5, 25, 6.25, 0.5},
	"claude-opus-4-6":   {5, 25, 6.25, 0.5},
	"claude-opus-4-5":   {5, 25, 6.25, 0.5},
	"claude-sonnet-5-5": {2, 10, 2.5, 0.10},
	"claude-sonnet-5":   {2, 10, 2.5, 0.2},
	"claude-sonnet-4":   {3, 15, 3.75, 0.3},
	"claude-haiku-5-5":  {0.10, 0.50, 0.125, 0.01},
	"claude-haiku-4-5":  {1, 5, 1.25, 0.1},
}

// priceFor finds the price of a model by the longest matching id prefix, so
// dated ids such as claude-haiku-4-5-20251001 still match.
func priceFor(model string) (Price, bool) {
	best, found := "", false
	for id := range prices {
		if strings.HasPrefix(model, id) && len(id) > len(best) {
			best, found = id, true
		}
	}
	return prices[best], found
}
