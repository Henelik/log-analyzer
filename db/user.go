package db

import (
	"fmt"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	Model

	Username string `gorm:"uniqueIndex;not null"`
	Password string `gorm:"not null"` // bcrypt hashed
}

// SeedUser creates a user with a bcrypt hashed password.  It is a no-op when a
// user with the same username already exists.
func SeedUser(database *gorm.DB, username, password string) error {
	var user User

	err := database.Where("username = ?", username).First(&user).Error
	if err == nil {
		return nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("look up user %q: %w", username, err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password for user %q: %w", username, err)
	}

	user = User{Username: username, Password: string(hashed)}

	if err := database.Create(&user).Error; err != nil {
		return fmt.Errorf("create user %q: %w", username, err)
	}

	return nil
}
