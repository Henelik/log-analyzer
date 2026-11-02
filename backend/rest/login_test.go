package rest

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	loginPath     = "/api/v1/login"
	testJWTSecret = "test-secret"
	testPassword  = "correct horse battery staple"
)

func newLoginApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()

	database := openTestDB(t)
	require.NoError(t, database.AutoMigrate(&db.User{}), "migrate login test database")

	return newApp(t, database), database
}

func seedUser(t *testing.T, database *gorm.DB, username string) db.User {
	t.Helper()

	hashed, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	require.NoError(t, err, "hash test password")

	user := db.User{Username: username, Password: string(hashed)}
	require.NoError(t, database.Create(&user).Error, "seed user %q", username)

	return user
}

func postLogin(t *testing.T, app *fiber.App, fields map[string]string) (int, []byte) {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		require.NoError(t, writer.WriteField(name, value), "write multipart field %q", name)
	}
	require.NoError(t, writer.Close(), "close multipart writer")

	req := httptest.NewRequest(http.MethodPost, loginPath, &body)
	req.Header.Set(fiber.HeaderContentType, writer.FormDataContentType())

	resp, err := app.Test(req)
	require.NoError(t, err, "post login")
	defer func() {
		require.NoError(t, resp.Body.Close(), "close login body")
	}()

	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read login body")

	return resp.StatusCode, payload
}

func validCredentials(username string) map[string]string {
	return map[string]string{
		loginUsernameField: username,
		loginPasswordField: testPassword,
	}
}

func parseToken(t *testing.T, token string) authClaims {
	t.Helper()

	var claims authClaims

	_, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) {
		return []byte(testJWTSecret), nil
	})
	require.NoError(t, err, "parse token")

	return claims
}

func TestLoginReturnsSignedToken(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, database := newLoginApp(t)
	user := seedUser(t, database, "alice")

	code, body := postLogin(t, app, validCredentials(user.Username))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[tokenResponse](t, body)
	require.NotEmpty(t, payload.Token, "token")

	claims := parseToken(t, payload.Token)

	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, user.Username, claims.Subject)
	assert.NotNil(t, claims.IssuedAt, "issued at")
	assert.NotNil(t, claims.ExpiresAt, "expires at")
	assert.True(t, claims.ExpiresAt.After(claims.IssuedAt.Time), "token expires after issue")
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, database := newLoginApp(t)
	user := seedUser(t, database, "alice")

	fields := validCredentials(user.Username)
	fields[loginPasswordField] = "wrong password"

	code, body := postLogin(t, app, fields)

	assert.Equal(t, http.StatusUnauthorized, code)
	requireUploadError(t, body, "invalid credentials")
}

func TestLoginRejectsUnknownUser(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, _ := newLoginApp(t)

	code, body := postLogin(t, app, validCredentials("nobody"))

	assert.Equal(t, http.StatusUnauthorized, code)
	requireUploadError(t, body, "invalid credentials")
}

func TestLoginSelectsRequestedUser(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, database := newLoginApp(t)
	seedUser(t, database, "alice")
	user := seedUser(t, database, "bob")

	code, body := postLogin(t, app, validCredentials(user.Username))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	claims := parseToken(t, decodeJSON[tokenResponse](t, body).Token)

	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, "bob", claims.Subject)
}

func TestLoginRequiresCredentials(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, _ := newLoginApp(t)

	testCases := map[string]map[string]string{
		"missing username": {loginPasswordField: testPassword},
		"missing password": {loginUsernameField: "alice"},
		"empty request":    {},
	}

	for name, fields := range testCases {
		t.Run(name, func(t *testing.T) {
			code, body := postLogin(t, app, fields)

			assert.Equal(t, http.StatusBadRequest, code)
			requireUploadError(t, body, "missing credentials")
		})
	}
}

func TestLoginRequiresSecret(t *testing.T) {
	t.Setenv(jwtSecretEnv, "")

	app, database := newLoginApp(t)
	user := seedUser(t, database, "alice")

	code, body := postLogin(t, app, validCredentials(user.Username))

	assert.Equal(t, http.StatusInternalServerError, code)
	requireUploadError(t, body, "cannot log in")
}

func TestLoginPasswordIsNotComparedVerbatim(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, database := newLoginApp(t)

	user := db.User{Username: "mallory", Password: testPassword}
	require.NoError(t, database.Create(&user).Error, "seed unhashed user")

	code, body := postLogin(t, app, validCredentials(user.Username))

	assert.Equal(t, http.StatusUnauthorized, code)
	requireUploadError(t, body, "invalid credentials")
}
