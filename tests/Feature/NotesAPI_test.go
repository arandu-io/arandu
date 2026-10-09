// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/auth"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/bootstrap"
)

// apiCall sends one request straight to the application's handler, with
// exactly the headers given and no cookie unless one is given: a program
// holding a token, not a browser.
func apiCall(t *testing.T, app bootstrap.App, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	answer := httptest.NewRecorder()
	app.Kernel.Handler().ServeHTTP(answer, request)
	return answer
}

// bearer is the two headers a token client sends with every call.
func bearer(token string) []string {
	return []string{"Authorization", "Bearer " + token, "Accept", "application/json"}
}

// problemStatus decodes a problem document and answers its status member.
func problemStatus(t *testing.T, answer *httptest.ResponseRecorder) int {
	t.Helper()
	if got := answer.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json; body:\n%s", got, answer.Body)
	}
	var problem struct {
		Status int    `json:"status"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &problem); err != nil || problem.Title == "" {
		t.Fatalf("the problem document does not decode with a title: %v\n%s", err, answer.Body)
	}
	return problem.Status
}

// TestATokenClientWritesWithNoSessionAndThePolicyStillDecides: a program that
// holds a personal access token writes a note with no session cookie and no
// CSRF token -- the CSRF check leaves a bearer request to RequireToken -- and
// what it may do is still the policy's answer, given about the account the
// token acts as.
func TestATokenClientWritesWithNoSessionAndThePolicyStillDecides(t *testing.T) {
	f := newNotesFixture(t)
	ctx := context.Background()
	token, _, err := f.app.Tokens.Issue(ctx, auth.Subject{ID: f.anaID, Tenant: bootstrap.Tenant()}, "notes client", 0)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}

	created := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"title":"From a script","body":"Posted with a token."}`, bearer(token)...)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST /api/notes answered %d, want 201:\n%s", created.Code, created.Body)
	}
	var one struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &one); err != nil {
		t.Fatalf("the answer is not the note's Resource: %v\n%s", err, created.Body)
	}
	address := created.Header().Get("Location")
	if !strings.HasPrefix(address, "/notes/") || one.Data["title"] != "From a script" {
		t.Fatalf("Location = %q and data = %v, want the note just written", address, one.Data)
	}
	if _, leaked := one.Data["tenant_id"]; leaked {
		t.Error("the answer carries the tenant")
	}
	stored := f.stored(t, address)
	if stored.UserID != f.anaID || stored.TenantID != bootstrap.Tenant() {
		t.Fatalf("the note was stored for user %q in tenant %q, want the token's account in its tenant", stored.UserID, stored.TenantID)
	}

	// The token is Ana, and Bea's note is not Ana's to publish: the policy
	// refuses it as it refuses Ana's browser.
	beas := f.write(t, f.bea, "Bea's plan")
	refused := apiCall(t, f.app, http.MethodPost, "/api"+beas+"/publish", "", bearer(token)...)
	if refused.Code != http.StatusForbidden || problemStatus(t, refused) != http.StatusForbidden {
		t.Fatalf("publishing another member's note with a token answered %d, want 403:\n%s", refused.Code, refused.Body)
	}
	if f.stored(t, beas).Published() {
		t.Fatal("the refused publication published the note")
	}

	// Ana's own is hers to publish.
	published := apiCall(t, f.app, http.MethodPost, "/api"+address+"/publish", "", bearer(token)...)
	if published.Code != http.StatusOK || !f.stored(t, address).Published() {
		t.Fatalf("publishing her own note with a token answered %d:\n%s", published.Code, published.Body)
	}

	// A rejected input is the same problem document a browser's JSON client
	// gets.
	untitled := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"body":"no title"}`, bearer(token)...)
	if untitled.Code != http.StatusUnprocessableEntity || problemStatus(t, untitled) != http.StatusUnprocessableEntity {
		t.Fatalf("a note with no title answered %d, want 422:\n%s", untitled.Code, untitled.Body)
	}
}

// TestATokenRouteRefusesWhatCarriesNoKnownToken: an unknown token and a
// revoked one are the same 401, and none of them writes. A write that carries
// no bearer token at all never reaches the guard: with no session either, it
// is the CSRF check's, and answers 419 like any other write without a token.
func TestATokenRouteRefusesWhatCarriesNoKnownToken(t *testing.T) {
	f := newNotesFixture(t)
	ctx := context.Background()
	ana := auth.Subject{ID: f.anaID, Tenant: bootstrap.Tenant()}
	revoked, row, err := f.app.Tokens.Issue(ctx, ana, "revoked", 0)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}
	if err := f.app.Tokens.Revoke(ctx, ana, row.ID); err != nil {
		t.Fatalf("revoking it: %v", err)
	}

	if bare := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"title":"Refused"}`, "Accept", "application/json"); bare.Code != 419 {
		t.Errorf("a write with no token at all answered %d, want 419:\n%s", bare.Code, bare.Body)
	}
	for name, headers := range map[string][]string{
		"unknown token": bearer("nobody-issued-this-token"),
		"revoked token": bearer(revoked),
	} {
		answer := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"title":"Refused"}`, headers...)
		if answer.Code != http.StatusUnauthorized || problemStatus(t, answer) != http.StatusUnauthorized {
			t.Errorf("%s: POST /api/notes answered %d, want 401:\n%s", name, answer.Code, answer.Body)
		}
		if got := answer.Header().Get("WWW-Authenticate"); got != "Bearer" {
			t.Errorf("%s: WWW-Authenticate = %q, want Bearer", name, got)
		}
	}
	if found, _ := models.Notes(f.app.DB).Where("title", "=", "Refused").
		First(ctx, auth.SystemGrant(policies.NoteView, bootstrap.Tenant())); found != nil {
		t.Fatal("a refused request stored a note")
	}
}

// TestARetriedTokenWriteIsAnsweredOnce: the same Idempotency-Key is answered
// from the store, marked as a replay, and writes one note.
func TestARetriedTokenWriteIsAnsweredOnce(t *testing.T) {
	f := newNotesFixture(t)
	ctx := context.Background()
	token, _, err := f.app.Tokens.Issue(ctx, auth.Subject{ID: f.anaID, Tenant: bootstrap.Tenant()}, "retrying client", 0)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}
	headers := append(bearer(token), "Idempotency-Key", "6b0f1c0e-retry-once")

	first := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"title":"Only once"}`, headers...)
	second := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"title":"Only once"}`, headers...)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("the two copies answered %d and %d, want 201 twice", first.Code, second.Code)
	}
	if second.Header().Get("Idempotent-Replayed") != "true" || second.Body.String() != first.Body.String() {
		t.Fatalf("the retry was not the first answer replayed: Idempotent-Replayed=%q\n%s\n%s",
			second.Header().Get("Idempotent-Replayed"), first.Body, second.Body)
	}
	written, err := models.Notes(f.app.DB).Where("title", "=", "Only once").
		Get(ctx, auth.SystemGrant(policies.NoteView, bootstrap.Tenant()))
	if err != nil {
		t.Fatalf("reading the notes: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("the retried write stored %d notes, want 1", len(written))
	}
}

// TestTheCSRFCheckStillGuardsWhatABrowserAttachesByItself: the bearer header
// is left to its guard only while nothing ambient rides along. A browser that
// reports the request as cross-site is refused whatever it carries, and a
// session cookie is checked for its CSRF token, bearer or not.
func TestTheCSRFCheckStillGuardsWhatABrowserAttachesByItself(t *testing.T) {
	f := newNotesFixture(t)
	ctx := context.Background()
	ana := auth.Subject{ID: f.anaID, Tenant: bootstrap.Tenant()}
	token, _, err := f.app.Tokens.Issue(ctx, ana, "cross-site", 0)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}

	crossSite := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"title":"Forged"}`,
		append(bearer(token), "Sec-Fetch-Site", "cross-site")...)
	if crossSite.Code != http.StatusForbidden {
		t.Errorf("a bearer write a browser reported as cross-site answered %d, want 403:\n%s", crossSite.Code, crossSite.Body)
	}

	// A real session cookie, started by the store the guards read.
	signIn := httptest.NewRecorder()
	if _, err := f.app.Sessions.Start(ctx, signIn, ana); err != nil {
		t.Fatalf("starting a session: %v", err)
	}
	cookie := signIn.Result().Cookies()[0]
	session := cookie.Name + "=" + cookie.Value

	withoutToken := apiCall(t, f.app, http.MethodPost, "/notes", `{"title":"Forged"}`,
		"Cookie", session, "Accept", "application/json")
	if withoutToken.Code != 419 {
		t.Errorf("a cookie write without a CSRF token answered %d, want 419:\n%s", withoutToken.Code, withoutToken.Body)
	}
	alongside := apiCall(t, f.app, http.MethodPost, "/api/notes", `{"title":"Forged"}`,
		append(bearer(token), "Cookie", session)...)
	if alongside.Code != 419 {
		t.Errorf("a bearer write riding a session cookie, without a CSRF token, answered %d, want 419:\n%s", alongside.Code, alongside.Body)
	}

	if found, _ := models.Notes(f.app.DB).Where("title", "=", "Forged").
		First(ctx, auth.SystemGrant(policies.NoteView, bootstrap.Tenant())); found != nil {
		t.Fatal("a refused request stored a note")
	}
}
