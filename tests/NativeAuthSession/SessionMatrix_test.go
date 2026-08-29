//go:build authui

package nativeauthsession_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	authui "github.com/arandu-io/arandu/app/Http/Controllers/Auth"
	"github.com/arandu-io/arandu/app/Models"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	twofactor "github.com/arandu-io/hesape/2fa"
	nativeauth "github.com/arandu-io/hesape/auth"
)

func TestPasswordAndPendingFactorSessionMatrix(t *testing.T) {
	t.Run("invalid password", func(t *testing.T) {
		harness := newSessionHarness(t)
		harness.users.verifyErr = nativeauth.ErrInvalidCredentials
		harness.post("/auth/login", url.Values{
			"email": {"ana@example.test"}, "password": {"wrong-password"},
		})
		harness.assertWrites(t, 0)
	})

	t.Run("valid password without factor", func(t *testing.T) {
		harness := newSessionHarness(t)
		harness.post("/auth/login", url.Values{
			"email": {"ana@example.test"}, "password": {"right-password"},
		})
		harness.assertWrites(t, 1)
	})

	t.Run("valid password with required factor", func(t *testing.T) {
		harness := newSessionHarness(t)
		harness.factors.required = true
		response := harness.post("/auth/login", url.Values{
			"email": {"ana@example.test"}, "password": {"right-password"},
		})
		harness.pendingCookie(t, response)
		harness.assertWrites(t, 0)
	})

	t.Run("invalid authenticator code", func(t *testing.T) {
		harness := newSessionHarness(t)
		pending := harness.startPending(t)
		harness.factors.verifyErr = twofactor.ErrInvalidCode
		harness.postWithCookies("/auth/two-factor/challenge", url.Values{
			"authenticator_code": {"123456"},
		}, pending)
		harness.assertWrites(t, 0)
	})

	t.Run("valid authenticator code", func(t *testing.T) {
		harness := newSessionHarness(t)
		pending := harness.startPending(t)
		harness.postWithCookies("/auth/two-factor/challenge", url.Values{
			"authenticator_code": {"123456"},
		}, pending)
		harness.assertWrites(t, 1)
	})

	t.Run("invalid recovery code", func(t *testing.T) {
		harness := newSessionHarness(t)
		pending := harness.startPending(t)
		harness.factors.recoveryErr = twofactor.ErrInvalidCode
		harness.postWithCookies("/auth/two-factor/recovery", url.Values{
			"recovery_code": {"ABCD-EFGH"},
		}, pending)
		harness.assertWrites(t, 0)
	})

	t.Run("valid recovery code", func(t *testing.T) {
		harness := newSessionHarness(t)
		pending := harness.startPending(t)
		harness.postWithCookies("/auth/two-factor/recovery", url.Values{
			"recovery_code": {"ABCD-EFGH"},
		}, pending)
		harness.assertWrites(t, 1)
	})

	t.Run("tampered pending cookie", func(t *testing.T) {
		harness := newSessionHarness(t)
		pending := harness.startPending(t)
		pending.Value += "tampered"
		harness.postWithCookies("/auth/two-factor/challenge", url.Values{
			"authenticator_code": {"123456"},
		}, pending)
		harness.assertWrites(t, 0)
	})

	t.Run("expired pending cookie", func(t *testing.T) {
		harness := newSessionHarness(t)
		payload, err := json.Marshal(map[string]any{
			"tenant": "tenant-a", "user_id": "user-a",
			"password_fingerprint": harness.users.credential.PasswordFingerprint(),
			"remember":             false,
		})
		if err != nil {
			t.Fatalf("encoding pending state: %v", err)
		}
		pending := &http.Cookie{
			Name:  "two-factor-pending",
			Value: security.NewSigner(harness.appKey).Sign("two-factor-pending", string(payload), -time.Second),
			Path:  "/auth/two-factor",
		}
		harness.postWithCookies("/auth/two-factor/challenge", url.Values{
			"authenticator_code": {"123456"},
		}, pending)
		harness.assertWrites(t, 0)
	})

	t.Run("password changed while pending", func(t *testing.T) {
		harness := newSessionHarness(t)
		pending := harness.startPending(t)
		harness.users.current.Password = "replacement-password-hash"
		harness.postWithCookies("/auth/two-factor/challenge", url.Values{
			"authenticator_code": {"123456"},
		}, pending)
		harness.assertWrites(t, 0)
	})
}

type sessionHarness struct {
	appKey  []byte
	users   *fakeUsers
	factors *fakeFactors
	backend *countingBackend
	handler http.Handler
}

func newSessionHarness(t *testing.T) *sessionHarness {
	t.Helper()

	appKey := []byte("0123456789abcdef0123456789abcdef")
	user := models.User{
		ID: "user-a", TenantID: "tenant-a", Name: "Ana", Email: "ana@example.test",
		Password: "stored-password-hash", Roles: []string{"member"},
	}
	users := &fakeUsers{credential: user, current: user}
	factors := &fakeFactors{}
	backend := &countingBackend{}
	sessions := security.NewSessionStore(appKey, time.Hour, false, backend)
	module := authui.New(
		users, factors, nil, sessions, security.NewCSRF(appKey, time.Hour), nil,
		appKey, "Arandu", authui.FixedTenant("tenant-a"), false,
	)
	router := fhttp.NewRouter()
	module.Routes(router)
	return &sessionHarness{
		appKey: appKey, users: users, factors: factors, backend: backend, handler: router,
	}
}

func (h *sessionHarness) startPending(t *testing.T) *http.Cookie {
	t.Helper()
	h.factors.required = true
	response := h.post("/auth/login", url.Values{
		"email": {"ana@example.test"}, "password": {"right-password"},
	})
	h.assertWrites(t, 0)
	return h.pendingCookie(t, response)
}

func (h *sessionHarness) post(path string, form url.Values) *httptest.ResponseRecorder {
	return h.postWithCookies(path, form)
}

func (h *sessionHarness) postWithCookies(path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *sessionHarness) pendingCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "two-factor-pending" && cookie.MaxAge > 0 {
			return cookie
		}
	}
	t.Fatal("the password-authenticated attempt wrote no pending cookie")
	return nil
}

func (h *sessionHarness) assertWrites(t *testing.T, want int) {
	t.Helper()
	h.backend.mu.Lock()
	defer h.backend.mu.Unlock()
	if h.backend.writes != want {
		t.Fatalf("session backend writes = %d, want %d", h.backend.writes, want)
	}
}

type countingBackend struct {
	mu     sync.Mutex
	writes int
}

func (*countingBackend) Get(context.Context, string) (security.Subject, error) {
	return security.Subject{}, security.ErrSessionExpired
}

func (b *countingBackend) Put(context.Context, string, security.Subject, time.Duration) error {
	b.mu.Lock()
	b.writes++
	b.mu.Unlock()
	return nil
}

func (*countingBackend) Delete(context.Context, string) error { return nil }

func (*countingBackend) DeleteSubject(context.Context, string, string, string) error { return nil }

type fakeUsers struct {
	credential models.User
	current    models.User
	verifyErr  error
}

func (*fakeUsers) PublicNames(context.Context, nativeauth.Subject, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (u *fakeUsers) VerifyCredentials(context.Context, string, string, string, string) (models.User, error) {
	return u.credential, u.verifyErr
}

func (u *fakeUsers) Register(context.Context, string, string, string, string) (models.User, error) {
	return u.credential, nil
}

func (u *fakeUsers) FindForAuthentication(context.Context, string, string) (models.User, error) {
	return u.current, nil
}

func (u *fakeUsers) Lookup(context.Context, string, string) (models.User, error) {
	return u.current, nil
}

func (u *fakeUsers) MarkVerified(context.Context, string, string, string) (models.User, bool, error) {
	return u.current, true, nil
}

func (u *fakeUsers) ResetPassword(context.Context, string, string, string, string, string) (models.User, error) {
	return u.current, nil
}

func (*fakeUsers) ConfirmPassword(context.Context, nativeauth.Subject, string, string) error {
	return nil
}

type fakeFactors struct {
	required    bool
	verifyErr   error
	recoveryErr error
}

func (f *fakeFactors) Required(context.Context, string, string) (bool, error) {
	return f.required, nil
}

func (*fakeFactors) Begin(context.Context, nativeauth.Subject, string) (twofactor.Provisioning, error) {
	return twofactor.Provisioning{}, errors.New("not used by the session matrix")
}

func (*fakeFactors) Confirm(context.Context, nativeauth.Subject, string) ([]string, error) {
	return nil, errors.New("not used by the session matrix")
}

func (*fakeFactors) Disable(context.Context, nativeauth.Subject) error {
	return errors.New("not used by the session matrix")
}

func (*fakeFactors) RegenerateRecoveryCodes(context.Context, nativeauth.Subject) ([]string, error) {
	return nil, errors.New("not used by the session matrix")
}

func (f *fakeFactors) VerifyAuthenticator(context.Context, string, string, string) error {
	return f.verifyErr
}

func (f *fakeFactors) ConsumeRecovery(context.Context, string, string, string) error {
	return f.recoveryErr
}
