package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestShouldRetryAllowsFallbackForCodexChannelAffinityAuthFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","prompt_cache_key":"pc-root-cause"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	_, found := service.GetPreferredChannelByAffinity(ctx, "gpt-5.5", "default")
	require.False(t, found, "cache miss still records the matched codex affinity rule metadata")
	require.True(t, service.ShouldSkipRetryAfterChannelAffinityFailure(ctx))

	upstream401 := types.NewErrorWithStatusCode(errors.New("token_expired"), types.ErrorCodeBadResponseStatusCode, http.StatusUnauthorized)
	require.True(t, shouldRetry(ctx, upstream401, 1), "upstream auth failures must fallback even when codex affinity is sticky")
}

func TestShouldRetrySkipsNonAuthFailureAfterCodexChannelAffinity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5","prompt_cache_key":"pc-root-cause"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	_, found := service.GetPreferredChannelByAffinity(ctx, "gpt-5.5", "default")
	require.False(t, found, "cache miss still records the matched codex affinity rule metadata")
	require.True(t, service.ShouldSkipRetryAfterChannelAffinityFailure(ctx))

	upstream500 := types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)
	require.False(t, shouldRetry(ctx, upstream500, 1), "non-auth failures should preserve sticky codex affinity behavior")
}

func TestShouldRetryRetriesUnauthorizedWithoutChannelAffinity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	upstream401 := types.NewErrorWithStatusCode(errors.New("token invalidated"), types.ErrorCodeBadResponseStatusCode, http.StatusUnauthorized)

	require.True(t, shouldRetry(ctx, upstream401, 1))
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}
