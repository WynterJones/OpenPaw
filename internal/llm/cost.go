package llm

import (
	"strconv"
)

type pricing struct {
	inputPerMillion  float64
	outputPerMillion float64
}

var fallbackPricing = map[string]pricing{
	ModelOpus:   {inputPerMillion: 5.00, outputPerMillion: 25.00},
	ModelSonnet: {inputPerMillion: 3.00, outputPerMillion: 15.00},
	ModelHaiku:  {inputPerMillion: 1.00, outputPerMillion: 5.00},
}

func CalculateCost(model string, inputTokens, outputTokens int64) float64 {
	// Try cache first
	if cached := globalModelCache.get(model); cached != nil {
		promptPrice, _ := strconv.ParseFloat(cached.Pricing.Prompt, 64)
		completionPrice, _ := strconv.ParseFloat(cached.Pricing.Completion, 64)
		if promptPrice > 0 || completionPrice > 0 {
			return float64(inputTokens)*promptPrice + float64(outputTokens)*completionPrice
		}
	}

	// Fallback to hardcoded pricing
	p, ok := fallbackPricing[model]
	if !ok {
		p = pricing{inputPerMillion: 3.00, outputPerMillion: 15.00}
	}
	return (float64(inputTokens)/1_000_000)*p.inputPerMillion +
		(float64(outputTokens)/1_000_000)*p.outputPerMillion
}
