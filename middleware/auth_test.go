package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAuthTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "alice")
		session.Set("role", common.RoleCommonUser)
		session.Set("id", 42)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/protected", UserAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt("id")})
	})
	return router
}

func TestUserAuthAllowsSessionWithoutNewAPIUserHeader(t *testing.T) {
	router := setupAuthTestRouter()

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
	router.ServeHTTP(loginRecorder, loginRequest)
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)

	protectedRecorder := httptest.NewRecorder()
	protectedRequest := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range loginRecorder.Result().Cookies() {
		protectedRequest.AddCookie(cookie)
	}
	router.ServeHTTP(protectedRecorder, protectedRequest)

	require.Equal(t, http.StatusOK, protectedRecorder.Code)
	require.JSONEq(t, `{"id":42}`, protectedRecorder.Body.String())
}

func TestUserAuthRejectsMismatchedExplicitNewAPIUserHeader(t *testing.T) {
	router := setupAuthTestRouter()

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
	router.ServeHTTP(loginRecorder, loginRequest)
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)

	protectedRecorder := httptest.NewRecorder()
	protectedRequest := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range loginRecorder.Result().Cookies() {
		protectedRequest.AddCookie(cookie)
	}
	protectedRequest.Header.Set("New-Api-User", "7")
	router.ServeHTTP(protectedRecorder, protectedRequest)

	require.Equal(t, http.StatusUnauthorized, protectedRecorder.Code)
	require.Contains(t, protectedRecorder.Body.String(), "mismatch")
}
