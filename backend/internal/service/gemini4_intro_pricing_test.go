package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGemini4PricesDoubleAfterPromotion(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	end := time.Date(2026, time.December, 1, 8, 0, 0, 0, time.UTC)
	before, after := end.Add(-time.Second), end
	for _, model := range []string{"gemini-4-argon", "gemini-4-argon-high", "gemini-4-carbon", "gemini-4-carbon-low", "models/gemini-4-carbon"} {
		t.Run(model, func(t *testing.T) {
			intro, err := svc.getModelPricingAt(model, before)
			require.NoError(t, err)
			require.InDelta(t, 2e-6, intro.InputPricePerToken, 1e-15)
			require.InDelta(t, 10e-6, intro.OutputPricePerToken, 1e-15)
			require.InDelta(t, 0.1e-6, intro.CacheReadPricePerToken, 1e-15)

			full, err := svc.getModelPricingAt(model, after)
			require.NoError(t, err)
			require.InDelta(t, 4e-6, full.InputPricePerToken, 1e-15)
			require.InDelta(t, 20e-6, full.OutputPricePerToken, 1e-15)
			require.InDelta(t, 0.2e-6, full.CacheReadPricePerToken, 1e-15)

			// A second pass (calculateTokenCost applies the policy again) must not double twice.
			again := svc.applyModelSpecificPricingPolicyEx(model, full, true, after)
			require.InDelta(t, 4e-6, again.InputPricePerToken, 1e-15)
		})
	}
}

func TestGemini4PromotionLeavesOtherCardsAlone(t *testing.T) {
	after := time.Date(2026, time.December, 2, 0, 0, 0, 0, time.UTC)
	card := &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6, InputPricePerTokenPriority: 3.6e-6, OutputPricePerTokenPriority: 18e-6}
	svc := NewBillingService(&config.Config{}, nil)
	// Priority rates double with the rest: $7.20 / $36.
	doubled := svc.applyModelSpecificPricingPolicyEx("gemini-4-carbon", card, true, after)
	require.InDelta(t, 7.2e-6, doubled.InputPricePerTokenPriority, 1e-15)
	require.InDelta(t, 36e-6, doubled.OutputPricePerTokenPriority, 1e-15)
	require.InDelta(t, 2e-6, card.InputPricePerToken, 1e-15, "the shared card is not mutated")
	// Operator (group/channel) prices, Argon Cyber Permissive and other models keep their price.
	require.Same(t, card, svc.applyModelSpecificPricingPolicyEx("gemini-4-carbon", card, false, after))
	require.Same(t, card, svc.applyModelSpecificPricingPolicyEx("gemini-4-argon-cyber-permissive", card, true, after))
	require.Same(t, card, svc.applyModelSpecificPricingPolicyEx("gemini-3.8-flash", card, true, after))
	// A card that already has the full rate is left as it is.
	full := &ModelPricing{InputPricePerToken: 4e-6, OutputPricePerToken: 20e-6}
	require.Same(t, full, svc.applyModelSpecificPricingPolicyEx("gemini-4-argon", full, true, after))
}
