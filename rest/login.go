package rest

import (
	"log"
	"os"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	loginUsernameField = "username"
	loginPasswordField = "password"

	jwtSecretEnv = "JWT_SECRET"
	jwtTTL       = 24 * time.Hour
)

type tokenResponse struct {
	Token string `json:"token"`
}

type authClaims struct {
	UserID uint `json:"user_id"`

	jwt.RegisteredClaims
}

// Login validates the submitted username and password and returns a signed JWT
// when the credentials match a user in the database.
func Login(database *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		username := c.FormValue(loginUsernameField)
		password := c.FormValue(loginPasswordField)

		if username == "" || password == "" {
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "missing credentials"})
		}

		secret := os.Getenv(jwtSecretEnv)
		if secret == "" {
			log.Printf("cannot log in: %s is not set", jwtSecretEnv)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot log in"})
		}

		var user db.User

		if err := database.Where("username = ?", username).First(&user).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("failed to get user %q: %v", username, err)

				return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot log in"})
			}

			return c.Status(fiber.StatusUnauthorized).JSON(uploadError{Error: "invalid credentials"})
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(uploadError{Error: "invalid credentials"})
		}

		now := time.Now()

		claims := authClaims{
			UserID: user.ID,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   user.Username,
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
			},
		}

		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			log.Printf("failed to sign token for user %q: %v", username, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot log in"})
		}

		return c.JSON(tokenResponse{Token: token})
	}
}
