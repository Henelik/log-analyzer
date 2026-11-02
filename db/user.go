package db

type User struct {
	Model

	Username string `gorm:"uniqueIndex;not null"`
	Password string `gorm:"not null"` // bcrypt hashed
}
