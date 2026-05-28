package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGlobalAPIRateLimitSkipsHermesDashboardProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalEnabled := common.GlobalApiRateLimitEnable
	originalNum := common.GlobalApiRateLimitNum
	originalDuration := common.GlobalApiRateLimitDuration
	originalRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = originalEnabled
		common.GlobalApiRateLimitNum = originalNum
		common.GlobalApiRateLimitDuration = originalDuration
		common.RedisEnabled = originalRedisEnabled
	})

	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 60
	common.RedisEnabled = false
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	router := gin.New()
	router.Use(GlobalAPIRateLimit())
	router.GET("/api/status", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/hermes/tenant/dashboard/assets/app.js", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/api/hermes/tenants/user/42/dashboard/api/status", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	require.Equal(t, http.StatusOK, performRateLimitRequest(router, "/api/status").Code)
	require.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/api/status").Code)
	require.Equal(t, http.StatusOK, performRateLimitRequest(router, "/api/hermes/tenant/dashboard/assets/app.js").Code)
	require.Equal(t, http.StatusOK, performRateLimitRequest(router, "/api/hermes/tenants/user/42/dashboard/api/status").Code)
}

func performRateLimitRequest(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "192.0.2.10:12345"
	router.ServeHTTP(recorder, req)
	return recorder
}
