package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type CustomClaims struct {
	jwt.RegisteredClaims
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
	Rol   string `json:"rol,omitempty"`
}

func (c *CustomClaims) GetRole() string {
	if c.Role != "" {
		return c.Role
	}
	return c.Rol
}

// RequireRole verifies the JWT token and enforces that the user's role is one of allowedRoles.
func RequireRole(jwtSecret string, allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization token required",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid Authorization header format. Expected 'Bearer <token>'",
			})
			return
		}

		tokenString := parts[1]
		claims := &CustomClaims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired authorization token",
			})
			return
		}

		userRole := claims.GetRole()

		if len(allowedRoles) > 0 {
			authorized := false
			for _, allowed := range allowedRoles {
				if strings.EqualFold(userRole, allowed) {
					authorized = true
					break
				}
			}

			if !authorized {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": fmt.Sprintf("Forbidden: role '%s' lacks required permissions", userRole),
				})
				return
			}
		}

		// Store user details in context
		c.Set("user_id", claims.Subject)
		c.Set("user_name", claims.Name)
		c.Set("user_email", claims.Email)
		c.Set("user_role", userRole)
		c.Set("auth_token", authHeader)

		c.Next()
	}
}

// GetUserContext extracts authenticated user details from Gin context
func GetUserContext(c *gin.Context) (uuid.UUID, string, error) {
	subVal, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, "", errors.New("missing user_id in context")
	}

	subStr, ok := subVal.(string)
	if !ok {
		return uuid.Nil, "", errors.New("user_id is not a string")
	}

	userID, err := uuid.Parse(subStr)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("invalid user uuid: %w", err)
	}

	nameVal, _ := c.Get("user_name")
	nameStr, _ := nameVal.(string)
	if nameStr == "" {
		nameStr = "Portero Desconocido"
	}

	return userID, nameStr, nil
}
