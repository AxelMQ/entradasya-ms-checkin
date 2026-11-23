package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func generateTestToken(secret, sub, name, role string, exp time.Duration) string {
	claims := CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			Issuer:    "entradasya-auth",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(exp)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Name:  name,
		Email: "test@entradasya.com",
		Role:  role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "test-secret-key-123"

	userID := uuid.New().String()
	porterToken := generateTestToken(secret, userID, "Portero Juan", "portero", 1*time.Hour)
	clientToken := generateTestToken(secret, uuid.New().String(), "Cliente Carlos", "cliente", 1*time.Hour)
	organizerToken := generateTestToken(secret, uuid.New().String(), "Organizador Ana", "organizador", 1*time.Hour)
	expiredToken := generateTestToken(secret, userID, "Portero Juan", "portero", -1*time.Hour)
	invalidSecretToken := generateTestToken("wrong-secret", userID, "Portero Juan", "portero", 1*time.Hour)

	setupRouter := func(allowedRoles ...string) *gin.Engine {
		r := gin.New()
		r.Use(RequireRole(secret, allowedRoles...))
		r.GET("/protected", func(c *gin.Context) {
			uid, name, err := GetUserContext(c)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"user_id":   uid.String(),
				"user_name": name,
			})
		})
		return r
	}

	tests := []struct {
		name           string
		allowedRoles   []string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "missing header",
			allowedRoles:   []string{"portero"},
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid format header",
			allowedRoles:   []string{"portero"},
			authHeader:     "Basic 123456",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "expired token",
			allowedRoles:   []string{"portero"},
			authHeader:     "Bearer " + expiredToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid secret",
			allowedRoles:   []string{"portero"},
			authHeader:     "Bearer " + invalidSecretToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "forbidden role",
			allowedRoles:   []string{"portero"},
			authHeader:     "Bearer " + clientToken,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "authorized porter role",
			allowedRoles:   []string{"portero"},
			authHeader:     "Bearer " + porterToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "multiple allowed roles - portero",
			allowedRoles:   []string{"portero", "organizador"},
			authHeader:     "Bearer " + porterToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "multiple allowed roles - organizador",
			allowedRoles:   []string{"portero", "organizador"},
			authHeader:     "Bearer " + organizerToken,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(tt.allowedRoles...)
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
