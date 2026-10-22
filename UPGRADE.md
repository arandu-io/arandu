# Upgrade guide

A project is a copy of this skeleton, taken by `aru new` on the day it was
created, and nothing pulls later releases into it. This file says, release by
release, what to change in a project cloned from an earlier skeleton to bring
it where a new one starts.

Only what a project has to do is listed. A change to the example resource, a
comment or a test that is the skeleton's own is not.

---

## v0.38.0 — a session's roles are read on every request, and a password reset revokes the tokens

This skeleton requires framework v0.62.1 and hesape v0.77.0. The session store
kept the roles written at sign-in, so an account that was demoted, deleted or
closed kept its access until the session ended. hesape v0.77.0 reads them
through a resolver the application installs, and refuses to write or read a
session with roles without one: **a project whose accounts carry roles makes
step 3 before it serves requests with the new release**, or every sign-in of
such an account, and every session one already holds, answers 500.

### 1. Require the releases

```sh
go get github.com/arandu-io/framework@v0.62.1 github.com/arandu-io/hesape@v0.77.0
go mod tidy
```

### 2. Migrate before the new binary serves

`aru migrate` adds the nullable `outbox.impersonator` column, which the outbox
names on every write. The migration ships with the events module; there is
nothing to copy into `database/migrations`. A project that registers hesape's
webhook module gets `webhook_client_calls.delivery_id` the same way.

### 3. The session resolver

Add `app/Services/CurrentSubject.go`, the one read of an account's current
subject -- through `User.Subject`, the builder a sign-in uses:

```go
var ErrAccountGone = errors.New("account: gone, closed or refused")

func CurrentSubject(ctx context.Context, users *account.Users, tenant, userID string) (auth.Subject, error) {
	found, err := users.FindForAuthentication(ctx, tenant, userID)
	if errors.Is(err, account.ErrUserNotFound) {
		return auth.Subject{}, ErrAccountGone
	}
	if err != nil {
		return auth.Subject{}, err
	}
	if err := users.Admit(ctx, found); err != nil {
		return auth.Subject{}, ErrAccountGone
	}
	user, ok := found.(*models.User)
	if !ok {
		return auth.Subject{}, fmt.Errorf("account: the account is a %T, not a *models.User", found)
	}
	return user.Subject(), nil
}
```

In `PersonalAccessTokenService.Resolve`, replace the lines from
`FindForAuthentication` to `return user.Subject(), nil` with the same read:

```go
subject, err := CurrentSubject(ctx, s.users, found.TenantID, found.UserID)
if errors.Is(err, ErrAccountGone) {
	return auth.Subject{}, ErrTokenNotAccepted
}
return subject, err
```

In `app/Providers/AppServiceProvider.go`, install the resolver once, after
`p.Users` is built, and add the method it names:

```go
d.Sessions.ResolveSubjects(p.currentSubject)
```

```go
func (p *AppServiceProvider) currentSubject(ctx context.Context, who auth.Subject) (auth.Subject, error) {
	subject, err := services.CurrentSubject(ctx, p.Users, who.Tenant, who.ID)
	if errors.Is(err, services.ErrAccountGone) {
		return auth.Subject{}, hhttp.ErrSubjectGone
	}
	return subject, err
}
```

with `hhttp "github.com/arandu-io/hesape/http"`. A session then answers the
account's roles, tenant and verification as they are stored on that request: a
deleted, closed or refused account ends the session and the guards send the
person to the sign-in screen, and a read that fails answers 503 instead of the
roles the session began with. It is one read of the account per authenticated
request.

Keep `User.Subject()` the only place a subject is made from a user. The auth
kit the project published signs people in through it from the kit release that
requires hesape v0.77.0; a project still running an earlier kit's
`subjectOf` republishes the kit.

### 4. A password reset revokes the tokens

In the same `account.Hooks`, beside `AfterClose`:

```go
AfterResetPassword: func(ctx context.Context, g auth.Grant, user account.Account) error {
	return p.Tokens.RevokeAll(ctx, g, user.AccountID())
},
```

It runs inside the reset's transaction, for a reset link and for
`Users.SetPassword`; `ChangePassword`, which proves the current password,
revokes nothing.

### 5. The tests

A session is now worth only what the stored account says. A test that signed
in a subject whose id is not in the users table is sent to the sign-in screen;
sign in an account the test created (`tests.Register`, `factories.Users`). A
test that signs in over `tests.Kernel`, whose database cannot be opened,
answers 503. `tests/Feature/FreshAuthorization_test.go` is the skeleton's proof
of step 3, and `TestAPasswordResetRevokesThePersonalAccessTokens` of step 4.

---

## v0.37.0 — production refuses the log mailer and an insecure cookie, and a missing APP_ENV is production

This skeleton requires framework v0.62.0 and hesape v0.76.0. Nothing in the
code a project wrote has to change; what changes is what a deployment has to
say, and a project that moves without saying it stops at the boot with a line
naming the variable.

### 1. Require the releases

```sh
go get github.com/arandu-io/framework@v0.62.0 github.com/arandu-io/hesape@v0.76.0
go mod tidy
```

### 2. The environment

- **A development machine names `APP_ENV=dev` in its `.env`.** An unset
  `APP_ENV` was dev and is production now, which turns off the debug console,
  the stack on the error page and the commands that empty a database. The
  `.env` copied from `.env.example` already has the line.
- **Production names a real mailer.** `MAIL_URL=log://`, and an unset
  `MAIL_URL`, which means the same, stop a production boot: the log transport
  writes every message, password-reset codes included, into the log. Set
  `smtp://`, `smtps://`, `resend://` or `sendgrid://`.
- **Production drops `SESSION_SECURE_COOKIE=false`.** It stops a production
  boot. A deployment served over http is a staging one: it sets
  `APP_ENV=staging`, where the value is still taken.
- **`aru migrate` runs before the new binary takes jobs.** On the database
  queue, a pop reads the new nullable `impersonator` column of the `jobs`
  table. The migration ships with the queue driver; there is nothing to copy
  into `database/migrations`.

### 3. The tests

A test that boots in production names the mailer a test reads,
`MAIL_URL=array://`, beside `APP_ENV=prod`; `tests.Kernel` does it for a
production kernel, and `productionEnv` in `tests/Feature/Commands_test.go` for
the commands. A case that declared `SESSION_SECURE_COOKIE=false` in production
declares it in staging, and `TestAnInsecureSessionCookieIsRefusedInProduction`
asserts the refusal. A test that named no environment and meant dev names it.

### Beside it

- A route that answers a path the framework mounts under `/_arandu/`, in a
  method the framework's route does not take, stops the boot. The usual cause
  is `r.Post("/", ...)`, whose pattern matches every path below it; write
  `"/{$}"`. This skeleton has none.
- A project that registers hesape's webhook module deletes the
  `ValidateCSRFTokensExcept` line for its receivers and the module's
  `RegisterJobHandlers` call in `WithJobs`: `Create` asks the module for both.
  This skeleton's `/webhooks/` is its own controller, so its line stays.
- A page that queued work, broadcast or cached a write while an operator
  viewed the account read-only now gets `auth.ErrReadOnly`; ask
  `Grant.ReadOnly()` to leave the action out of the page.

Then `aru view:build`, `go test ./...` and `aru doctor`.

---

## v0.35.1 — closing an account takes its tokens away

`Users.Close` released the address and locked the password, and every personal
access token the account had issued went on acting as it. hesape v0.63.0 gives
the account rules `Hooks.AfterClose`, which runs inside the close's
transaction, and `Users.Admit`, which answers whether an account may act
through a credential other than its password. This skeleton uses both; a
project created from v0.35.0 makes the same three changes.

### 1. Require the releases

```sh
go get github.com/arandu-io/framework@v0.57.1 github.com/arandu-io/hesape@v0.63.0
go mod tidy
```

### 2. The token service

In `app/Services/PersonalAccessTokenService.go`, add the method the close
calls:

```go
func (s *PersonalAccessTokenService) RevokeAll(ctx context.Context, closing auth.Grant, userID string) error {
	if err := closing.Check(account.ActionUserUpdate); err != nil {
		return err
	}
	_, err := models.PersonalAccessTokens(s.db).Where("user_id", "=", userID).Delete(ctx, closing)
	return err
}
```

and in `Resolve`, right after `FindForAuthentication` answers the owner, refuse
an account the rules refuse:

```go
if err := s.users.Admit(ctx, owner); err != nil {
	return auth.Subject{}, ErrTokenNotAccepted
}
```

`Admit` answers nothing but a refusal -- a closed account, or one the
project's `Hooks.Admit` refuses -- so every error it returns is the token's
`ErrTokenNotAccepted`.

### 3. The wiring

In `bootstrap/app.go`, declare the token service before the account rules,
hand the close to it, and assign it where it was built with `:=`:

```go
var tokens *services.PersonalAccessTokenService
users, err := account.NewUsers(account.Config{
	// ...
	Hooks: account.Hooks{
		AfterClose: func(ctx context.Context, g auth.Grant, user account.Account) error {
			return tokens.RevokeAll(ctx, g, user.AccountID())
		},
	},
})
// ...
tokens = services.NewPersonalAccessTokenService(db, users, cfg.Auth.Tenant)
```

A project that already sets `Hooks` keeps its other hooks beside this one. One
whose own credentials act as an account -- a remembered device, a session read
back from a store -- deletes them in the same hook and asks `Users.Admit` where
it resolves them.

`tests/Feature/PersonalAccessTokens_test.go`'s
`TestAClosedAccountActsThroughNoToken` is the test.

### Beside it

- A project that published the sign-in screens republishes them for ui
  v0.24.2, which signs in a password stored with a trailing space without
  rewriting its hash: `go run github.com/arandu-io/ui@v0.24.2 auth --force`,
  as ui's
  [UPGRADE.md for v0.24.2](https://github.com/arandu-io/ui/blob/main/UPGRADE.md)
  says.
- A project whose forms carry personal fields -- a CPF, a mobile number --
  keeps them out of the flash cookie by naming them once, at boot, on the
  router `routes.Web` receives: `r.Flash().Withhold("cpf", "mobile")` in
  `routes/web.go`. This skeleton has none.

Then `aru view:build`, `go test ./...` and `aru doctor`.

---

## v0.35.0 — the account rules are `hesape/auth/account`

Projects created from v0.34.4 or earlier carry their own copy of the account
and second-factor rules: `UserService`, `TwoFactorService` and the files around
them. Since hesape v0.60.0 those rules are a package,
`github.com/arandu-io/hesape/auth/account`, and this skeleton builds them from
it. What stays the project's is the user model, the user policy, the users
migrations and the screens.

Nothing in the database changes. The package reads and writes the rows the copy
wrote -- the `user_two_factor` and `user_recovery_codes` tables, the sealed TOTP
secrets, the recovery-code hashes, the password fingerprint a reset code or a
pending sign-in carries, and every event name and payload -- so a code issued
before the upgrade is accepted after it.

The procedure below is the skeleton's half. hesape's
[UPGRADE.md for v0.60.0](https://github.com/arandu-io/hesape/blob/main/UPGRADE.md)
describes the package, and ui's
[UPGRADE.md for v0.24.0](https://github.com/arandu-io/ui/blob/main/UPGRADE.md)
the sign-in screens that call it, including the hooks a project with its own
sign-up fields uses instead of a copy.

### 1. Require the releases

```sh
go get github.com/arandu-io/framework@v0.57.0 github.com/arandu-io/hesape@v0.60.0
go mod tidy
```

and build with `aru` v0.73.0: `ARG ARU_VERSION=v0.73.0` in the `Dockerfile`,
and the three mentions of the version in `AGENTS.md`.

### 2. Delete the copy

```sh
git rm app/Services/UserService.go app/Services/TwoFactorService.go \
  app/Repositories/TwoFactorRepository.go \
  app/Models/TwoFactor.go app/Models/TwoFactorQuery.go app/Models/RecoveryCodeQuery.go \
  app/Policies/TwoFactorPolicy.go app/Events/User.go
```

`app/Events/User.go` held only the auth event names and their payload; they are
`account.UserRegistered`, `account.EmailVerified` and the rest, with
`account.UserEvent` as the payload. A project that added its own events to that
file moves them to a file of their own first.

### 3. The user model

`*models.User` implements `account.Account`. In `app/Models/User.go`:

- add the six readers, `AccountID`, `AccountTenant`, `AccountEmail`,
  `AccountName`, `PasswordHash` and `IsVerified`, each returning its column, and
  `var _ account.Account = (*User)(nil)` beside them;
- rename `Verified()` to `IsVerified()`, in `Subject()` and wherever else it is
  called;
- delete `PasswordFingerprint()`: the function is `account.Fingerprint(hash)`;
- delete the `GetQuery` scope in the custom block, which only the copy's
  credential provider called.

### 4. The user policy

`UserPolicy` decides over `*models.User` -- the pointer the users query builds,
which is what `account.PolicyOf` hands it:

```go
func (UserPolicy) Can(_ context.Context, subject auth.Subject, action auth.Action, user *models.User) error

var _ auth.Policy[*models.User] = UserPolicy{}
```

Delete the `ActionUser*` constants and compare against `account.ActionUserView`,
`account.ActionUserCreate`, `account.ActionUserUpdate`,
`account.ActionUserDelete` and `account.ActionUserNamesPublic`. They are the
same strings; every `policies.ActionUser*` elsewhere, a seeder or a test
included, becomes `account.ActionUser*`.

### 5. The second factor's migration

Keep `database/migrations/20260828_0003_create_two_factor.go`, or replace it
with the package's, the way this skeleton does: delete the file and add
`database/migrations/account.go`:

```go
package migrations

import (
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/database/migrations"
)

func init() {
	for _, migration := range account.Migrations() {
		migrations.Register(migration)
	}
}
```

The package's migration has the same name and the same schema, so a database
that applied the old one sees nothing pending. Register one or the other,
never both: two migrations under one name stop the binary when it starts.

### 6. The wiring

In `bootstrap/app.go`, build the outbox once, hand it to the relay, and build
the account rules over it where `NewUserService` and `NewTwoFactorService` were:

```go
outbox := events.NewOutbox(db)
relay := events.NewRelay(outbox, listeners.Each{ /* ... */ }, events.RelayOptions{Locker: locker})

users, err := account.NewUsers(account.Config{
	DB:     db,
	Outbox: outbox,
	Query:  func() *model.Builder { return models.Users(db).Base() },
	Policy: account.PolicyOf(policies.UserPolicy{}),
})
if err != nil {
	return App{}, fmt.Errorf("bootstrap: build the account rules: %w", err)
}
factors, err := account.NewFactors(db, fw.App.Key, limitStore.GetStore(), outbox, users)
if err != nil {
	return App{}, fmt.Errorf("bootstrap: build the second factor: %w", err)
}
```

`App.Users` becomes `*account.Users` and `App.TwoFactor` `*account.Factors`.
`users` goes wherever `userService` went -- `controllers.NewHomeController`,
`services.NewPersonalAccessTokenService` -- and `factors` wherever
`twoFactorService` did. A project that published the sign-in screens passes
both to `authui.New`; its signature did not change.

A rule the project had added to its copy is a hook on `account.Config.Hooks`:
its own columns at registration are `Prepare`, what it creates beside a new
account is `AfterCreate`, a suspension is `Admit`.

### 7. The callers

Every method answers an `account.Account`, which is the project's
`*models.User`; a caller that needs the type asserts it once.

- `app/Services/PersonalAccessTokenService.go` takes `*account.Users`, compares
  with `account.ErrUserNotFound`, and builds the subject from the
  `*models.User` that `FindForAuthentication` returns.
- `database/seeders/seeders.go`: `Deps.Users` is `*account.Users`.
- `database/seeders/UserSeeder.go`: `EnsureUser` is `Provision`, with the roles
  written by `Draft.Fill`:

  ```go
  user, err := d.Users.Provision(ctx, d.Tenant, account.Draft{
  	Name: name, Email: email, Password: password, Verified: true,
  	Fill: func(a account.Account) error {
  		a.(*models.User).Roles = append(models.Roles(nil), roles...)
  		return nil
  	},
  })
  ```

  and `services.ErrUserNotFound` is `account.ErrUserNotFound`.
- `Register(ctx, tenant, name, email, password)` is
  `Register(ctx, tenant, account.Registration{Name: name, Email: email, Password: password})`.

### 8. The tests

The tests that exercised the copy -- `tests/NativeAuth`, and the source checks
of the deleted files in `tests/AuthContracts` -- are replaced by the package's
contract suite, run against the project's own users table, model and policy:

```go
func TestAccountContract(t *testing.T) {
	accounttest.Run(t, func(t *testing.T) accounttest.Fixture {
		db := openAMigratedDatabase(t) // the outbox, the users migrations, account.Migrations()
		return accounttest.Fixture{
			DB:     db,
			Query:  func() *model.Builder { return models.Users(db).Base() },
			Policy: account.PolicyOf(policies.UserPolicy{}),
		}
	})
}
```

This skeleton's `tests/NativeAuth/AccountContract_test.go` is that test, and
`tests/AuthContracts/Boundaries_test.go` holds the guard that keeps the copy
from coming back.

### 9. Republish the screens

A project that published the sign-in screens republishes them against the
package:

```sh
go run github.com/arandu-io/ui@v0.24.0 auth --force
```

Then `aru view:build`, `go test ./...` and `aru doctor`.
