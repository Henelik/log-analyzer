package rest

import (
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

const (
	authScheme       = "Bearer"
	authErrorMessage = "unauthorized"
	authConfigError  = "cannot authenticate"
)

// authClaimsKey is the request-local key holding the claims RequireAuth verified.
type authClaimsKey struct{}

// RequireAuth rejects requests without a valid JWT signed by this server.  The
// verified claims are stored on the request for downstream handlers.  Register
// it before any route that must be protected.
func RequireAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		secret := os.Getenv(jwtSecretEnv)
		if secret == "" {
			log.Printf("cannot authenticate request: %s is not set", jwtSecretEnv)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: authConfigError})
		}

		tokenString, ok := bearerToken(c.Get(fiber.HeaderAuthorization))
		if !ok {
			return unauthorized(c)
		}

		var claims authClaims

		token, err := jwt.ParseWithClaims(
			tokenString,
			&claims,
			func(*jwt.Token) (any, error) {
				return []byte(secret), nil
			},
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || !token.Valid {
			return unauthorized(c)
		}

		c.SetLocal(authClaimsKey{}, claims)

		return c.Next()
	}
}

func unauthorized(c fiber.Ctx) error {
	c.Set(fiber.HeaderWWWAuthenticate, authScheme)

	return c.Status(fiber.StatusUnauthorized).JSON(uploadError{Error: authErrorMessage})
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, authScheme) {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	return token, true
}
