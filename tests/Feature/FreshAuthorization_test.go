package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arandu-io/hesape/auth"
	accounts "github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/http/middleware"

	models "github.com/arandu-io/arandu/app/Models"
	appconfig "github.com/arandu-io/arandu/config"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// A session keeps who signed in, and every request asks the account what it
// may do now. These tests change the stored account behind a browser that is
// already signed in and read the answer on the very next request.

// adminArea answers what an address guarded by RequireRole for the admin role
// -- the guard the /admin group in the comment of routes/web.go is mounted
// with -- answers a request carrying the session cookies: 204 when the guard
// lets it through. The application mounts no such group, so the guard is
// built here over the store the application's guards read.
func adminArea(t *testing.T, server *tests.Server, session []*http.Cookie) int {
	t.Helper()
	guarded := middleware.RequireRole(server.App.Sessions, models.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admin", nil)
	for _, c := range session {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)
	return rec.Code
}

// setRoles writes the roles column of one account directly, the way an
// administrator's screen or a console command would change it, with nothing
// told to the session.
func setRoles(t *testing.T, app tests.Application, userID, roles string) {
	t.Helper()
	if _, err := app.DB.ExecContext(context.Background(), `UPDATE users SET roles = ? WHERE id = ?`, roles, userID); err != nil {
		t.Fatalf("writing the roles of %s: %v", userID, err)
	}
}

// TestDemotingARoleRemovesAccessOnTheNextRequest signs an administrator in,
// takes the role away in the database, and asks again: the session that was
// opened with the role no longer carries it. Giving the role back opens the
// area on the next request too, so the roles are read and not merely dropped.
func TestDemotingARoleRemovesAccessOnTheNextRequest(t *testing.T) {
	server := tests.Serve(t)
	admin, err := factories.Users(server.App.DB).State(func(u *models.User) {
		u.Roles = models.Roles{models.RoleAdmin}
	}).CreateOne(context.Background(), auth.SystemGrant(accounts.ActionUserCreate, appconfig.Tenant()))
	if err != nil {
		t.Fatalf("creating the administrator: %v", err)
	}

	session := tests.SignIn(t, server, admin).Cookies()
	if code := adminArea(t, server, session); code != http.StatusNoContent {
		t.Fatalf("the administrator was answered %d by the admin area, want 204", code)
	}

	setRoles(t, server.App, admin.ID, `["member"]`)
	if code := adminArea(t, server, session); code != http.StatusForbidden {
		t.Fatalf("the demoted account was answered %d by the admin area, want 403", code)
	}

	setRoles(t, server.App, admin.ID, `["admin"]`)
	if code := adminArea(t, server, session); code != http.StatusNoContent {
		t.Fatalf("the account given the role back was answered %d by the admin area, want 204", code)
	}
}

// TestRemovingTheAccountEndsTheSession removes the account behind a signed-in
// browser, by deleting its row and by closing it, and the next request to a
// guarded page is sent to the sign-in screen rather than served.
func TestRemovingTheAccountEndsTheSession(t *testing.T) {
	const password = "a-long-enough-password"
	for name, remove := range map[string]func(t *testing.T, app tests.Application, user *models.User){
		"its row is deleted": func(t *testing.T, app tests.Application, user *models.User) {
			if _, err := app.DB.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, user.ID); err != nil {
				t.Fatalf("deleting the account: %v", err)
			}
		},
		"it is closed": func(t *testing.T, app tests.Application, user *models.User) {
			if err := app.Users.Close(context.Background(), user.Subject(), password, "203.0.113.7"); err != nil {
				t.Fatalf("closing the account: %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := tests.Serve(t)
			user := tests.Register(t, server.App, "Ana Lima", "ana@example.test", password)
			browser := tests.SignIn(t, server, user)
			browser.Get("/notes").AssertOk()

			remove(t, server.App, user)

			browser.Get("/notes").AssertRedirect(middleware.SignInPath)
		})
	}
}

// TestAnAccountThatCannotBeReadAnswers503 makes the account unreadable behind
// a signed-in browser: the request fails with 503 instead of being served with
// the roles the session began with, or treated as a sign-out. When the account
// can be read again the same session works, because a failed read is not the
// account going away.
func TestAnAccountThatCannotBeReadAnswers503(t *testing.T) {
	server := tests.Serve(t)
	app := server.App
	user := tests.Register(t, app, "Ana Lima", "ana@example.test", "a-long-enough-password")
	browser := tests.SignIn(t, server, user)
	browser.Get("/notes").AssertOk()

	ctx := context.Background()
	if _, err := app.DB.ExecContext(ctx, `ALTER TABLE users RENAME TO users_unreachable`); err != nil {
		t.Fatalf("hiding the users table: %v", err)
	}
	browser.Get("/notes").AssertServiceUnavailable().AssertHeaderMissing("Location")

	if _, err := app.DB.ExecContext(ctx, `ALTER TABLE users_unreachable RENAME TO users`); err != nil {
		t.Fatalf("putting the users table back: %v", err)
	}
	browser.Get("/notes").AssertOk()
}
