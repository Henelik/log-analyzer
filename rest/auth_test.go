package rest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func signToken(t *testing.T, method jwt.SigningMethod, secret string, claims authClaims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	require.NoError(t, err, "sign token")

	return token
}

func validAuthClaims() authClaims {
	now := time.Now()

	return authClaims{
		UserID: 7,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
}

// authorizeRequest configures the test secret and attaches a valid token to req.
func authorizeRequest(t *testing.T, req *http.Request) *http.Request {
	t.Helper()

	t.Setenv(jwtSecretEnv, testJWTSecret)
	req.Header.Set(fiber.HeaderAuthorization, authScheme+" "+signToken(t, jwt.SigningMethodHS256, testJWTSecret, validAuthClaims()))

	return req
}

func testRequest(t *testing.T, app *fiber.App, method, path, authorization string) (int, []byte) {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	if authorization != "" {
		req.Header.Set(fiber.HeaderAuthorization, authorization)
	}

	resp, err := app.Test(req)
	require.NoError(t, err, "%s %s", method, path)
	defer func() {
		require.NoError(t, resp.Body.Close(), "close %s %s body", method, path)
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read %s %s body", method, path)

	return resp.StatusCode, body
}

func TestRequireAuthRejectsRequestsWithoutToken(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, database := newFilesApp(t)
	file := seedFiles(t, database)[0]

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, uploadPath},
		{http.MethodPost, uploadPath},
		{http.MethodGet, filePath(file.ID)},
		{http.MethodDelete, filePath(file.ID)},
		{http.MethodGet, detectionsPath(file.ID)},
		{http.MethodGet, filePath(file.ID) + "/errors"},
		{http.MethodPost, eventPath(file.ID)},
	}

	for _, route := range protected {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			code, body := testRequest(t, app, route.method, route.path, "")

			assert.Equal(t, http.StatusUnauthorized, code)
			requireUploadError(t, body, authErrorMessage)
		})
	}
}

func TestRequireAuthRejectsInvalidTokens(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	now := time.Now()

	tests := map[string]func(t *testing.T) string{
		"malformed token": func(t *testing.T) string {
			return "not-a-token"
		},
		"wrong secret": func(t *testing.T) string {
			return signToken(t, jwt.SigningMethodHS256, "other-secret", validAuthClaims())
		},
		"expired token": func(t *testing.T) string {
			claims := validAuthClaims()
			claims.IssuedAt = jwt.NewNumericDate(now.Add(-2 * time.Hour))
			claims.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Hour))

			return signToken(t, jwt.SigningMethodHS256, testJWTSecret, claims)
		},
		"missing expiration": func(t *testing.T) string {
			claims := validAuthClaims()
			claims.ExpiresAt = nil

			return signToken(t, jwt.SigningMethodHS256, testJWTSecret, claims)
		},
		"unexpected signing method": func(t *testing.T) string {
			return signToken(t, jwt.SigningMethodHS512, testJWTSecret, validAuthClaims())
		},
		"unsigned token": func(t *testing.T) string {
			token, err := jwt.NewWithClaims(jwt.SigningMethodNone, validAuthClaims()).
				SignedString(jwt.UnsafeAllowNoneSignatureType)
			require.NoError(t, err, "sign none token")

			return token
		},
	}

	for name, makeToken := range tests {
		t.Run(name, func(t *testing.T) {
			app, _ := newFilesApp(t)

			code, body := testRequest(t, app, http.MethodGet, uploadPath, authScheme+" "+makeToken(t))

			assert.Equal(t, http.StatusUnauthorized, code)
			requireUploadError(t, body, authErrorMessage)
		})
	}
}

func TestRequireAuthRejectsMalformedAuthorizationHeader(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	token := signToken(t, jwt.SigningMethodHS256, testJWTSecret, validAuthClaims())

	headers := map[string]string{
		"wrong scheme": "Basic " + token,
		"bare token":   token,
		"empty token":  authScheme + " ",
	}

	for name, header := range headers {
		t.Run(name, func(t *testing.T) {
			app, _ := newFilesApp(t)

			code, body := testRequest(t, app, http.MethodGet, uploadPath, header)

			assert.Equal(t, http.StatusUnauthorized, code)
			requireUploadError(t, body, authErrorMessage)
		})
	}
}

func TestRequireAuthSetsChallengeHeader(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, _ := newFilesApp(t)

	req := httptest.NewRequest(http.MethodGet, uploadPath, nil)

	resp, err := app.Test(req)
	require.NoError(t, err, "get files")
	defer func() {
		require.NoError(t, resp.Body.Close(), "close body")
	}()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, authScheme, resp.Header.Get(fiber.HeaderWWWAuthenticate))
}

func TestRequireAuthAllowsValidToken(t *testing.T) {
	app, _ := newFilesApp(t)

	req := authorizeRequest(t, httptest.NewRequest(http.MethodGet, uploadPath, nil))

	resp, err := app.Test(req)
	require.NoError(t, err, "get files")
	defer func() {
		require.NoError(t, resp.Body.Close(), "close body")
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body")

	assert.Equal(t, http.StatusOK, resp.StatusCode, "body %q", body)
	assert.JSONEq(t, `{"files":[]}`, string(body))
}

func TestRequireAuthRequiresSecret(t *testing.T) {
	t.Setenv(jwtSecretEnv, "")

	app, _ := newFilesApp(t)

	code, body := testRequest(t, app, http.MethodGet, uploadPath,
		authScheme+" "+signToken(t, jwt.SigningMethodHS256, testJWTSecret, validAuthClaims()))

	assert.Equal(t, http.StatusInternalServerError, code)
	requireUploadError(t, body, authConfigError)
}

func TestRequireAuthExposesClaimsToHandlers(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app := fiber.New()
	app.Use(RequireAuth())
	app.Get("/me", func(c fiber.Ctx) error {
		claims, ok := c.Locals(authClaimsKey{}).(authClaims)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "missing claims"})
		}

		return c.JSON(fiber.Map{"user_id": claims.UserID, "subject": claims.Subject})
	})

	req := authorizeRequest(t, httptest.NewRequest(http.MethodGet, "/me", nil))

	resp, err := app.Test(req)
	require.NoError(t, err, "get me")
	defer func() {
		require.NoError(t, resp.Body.Close(), "close body")
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read body")

	require.Equal(t, http.StatusOK, resp.StatusCode, "body %q", body)
	assert.JSONEq(t, `{"user_id":7,"subject":"alice"}`, string(body))
}

func TestLoginIsPubliclyReachable(t *testing.T) {
	t.Setenv(jwtSecretEnv, testJWTSecret)

	app, database := newLoginApp(t)
	user := seedUser(t, database, "alice")

	code, body := postLogin(t, app, validCredentials(user.Username))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	assert.NotEmpty(t, decodeJSON[tokenResponse](t, body).Token)
}
