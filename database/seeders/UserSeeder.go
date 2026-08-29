package seeders

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/arandu-io/arandu/app/Services"
)

// UserSeeder creates or repairs one account, named on the command line.
//
//	aru db:seed UserSeeder -e you@example.com -p a-long-password
//	aru db:seed UserSeeder -e you@example.com -p a-long-password -r admin
//	aru db:seed UserSeeder -upd -e you@example.com -p a-new-password
//
// It is the operator's door, and it exists because the alternative keeps coming
// up: the first administrator of a fresh deployment, or somebody locked out
// before the mail transport is configured, cannot use the reset link -- there is
// nowhere for it to go.
//
// # The password is on the command line, and that has a cost
//
// It lands in the shell history and it is visible in `ps` to anybody on the
// machine while the command runs. That is a real exposure and it is the reason
// the environment-variable form existed. It is stated on stdout every time this
// runs, rather than in a comment nobody reads, and the command prints how to
// clear the history line.
//
// The mitigation that does not need a second way to pass a secret: run it, then
// change the password from the application. This command is for getting in.
//
// # Why -upd is a flag and not the default
//
// Creating is safe to repeat and updating is not: a seeder that silently reset
// the password of an existing account would lock somebody out the second time
// somebody ran `aru db:seed` after adding this. Without -upd, an address that is
// already there is left exactly as it is, and the command says so.
type UserSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (UserSeeder) Name() string { return "UserSeeder" }

// Run creates the account, or replaces its password when -upd is given.
func (UserSeeder) Run(ctx context.Context, d Deps) error {
	if d.Users == nil {
		return errors.New("the user service is not wired")
	}
	if d.Tenant == "" {
		return errors.New("the tenant is not wired: seeding into an empty tenant would create a user nobody can log in as")
	}

	email, _ := Flag(d.Args, "e")
	if email == "" {
		email, _ = Flag(d.Args, "email")
	}
	password, _ := Flag(d.Args, "p")
	if password == "" {
		password, _ = Flag(d.Args, "password")
	}
	name, _ := Flag(d.Args, "n")
	if name == "" {
		name, _ = Flag(d.Args, "name")
	}

	if email == "" || password == "" {
		return errors.New(`-e and -p are both required.

    aru db:seed UserSeeder -e you@example.com -p a-long-password
    aru db:seed UserSeeder -upd -e you@example.com -p a-new-password

Add -r admin to give the account the administrator role.`)
	}

	// Roles are repeatable, and the flag is read by hand for that: Flag returns
	// the first match, which is the right answer for an address and the wrong
	// one for a list.
	roles := rolesFrom(d.Args)

	if name == "" {
		// The local part of the address. Better than empty -- the name is what a
		// comment is signed with -- and obviously a placeholder, so nobody
		// mistakes it for something somebody chose.
		name, _, _ = strings.Cut(email, "@")
	}

	warnAboutTheShell()

	// Asked before anything is written, so "created" and "already there" are two
	// answers rather than one guess about a timestamp.
	existing, err := d.Users.Lookup(ctx, d.Tenant, email)
	switch {
	case errors.Is(err, services.ErrUserNotFound):
		// Created verified. Nobody is going to click a link in the inbox of an
		// account made from a terminal to unlock a deployment, and an
		// administrator who cannot moderate because of it is a first run that
		// ends at the sign-in screen.
		user, err := d.Users.EnsureUser(ctx, d.Tenant, name, email, password, roles, true)
		if err != nil {
			return err
		}
		fmt.Printf("created %s%s\n", user.Email, describeRoles(user.Roles))
		return nil

	case err != nil:
		return fmt.Errorf("looking the address up: %w", err)
	}

	if !Switch(d.Args, "upd") && !Switch(d.Args, "update") {
		// Left alone, and said out loud. A command that silently did nothing is
		// a command somebody runs three times before reading the output.
		fmt.Printf("%s already exists and was left as it is%s\n", existing.Email, describeRoles(existing.Roles))
		fmt.Printf("  to replace the password: aru db:seed UserSeeder -upd -e %s -p <password>\n", existing.Email)
		return nil
	}

	updated, err := d.Users.SetPassword(ctx, d.Tenant, email, password)
	if err != nil {
		return fmt.Errorf("replacing the password: %w", err)
	}
	// The roles are NOT changed here, and -r is ignored on an update. Granting
	// an existing account a role is a different decision from resetting its
	// password, and doing both from one flag is how somebody typing a password
	// reset makes an administrator by accident.
	if len(roles) > 0 {
		fmt.Println("  -r was ignored: this replaces the password and nothing else")
	}
	fmt.Printf("password replaced for %s%s\n", updated.Email, describeRoles(updated.Roles))
	return nil
}

// rolesFrom collects every -r, so an account can hold more than one.
func rolesFrom(args []string) []string {
	var out []string
	for i, a := range args {
		switch {
		case (a == "-r" || a == "--role") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-"):
			out = append(out, args[i+1])
		default:
			for _, prefix := range []string{"-r=", "--role="} {
				if value, ok := strings.CutPrefix(a, prefix); ok && value != "" {
					out = append(out, value)
				}
			}
		}
	}
	return out
}

func describeRoles(roles []string) string {
	if len(roles) == 0 {
		return ", with no roles"
	}
	return ", with " + strings.Join(roles, " and ")
}

// warnAboutTheShell says the thing the comment on this type says, on stdout,
// where it is read.
//
// Once per run and not per line. A warning that scrolls is a warning nobody
// finishes reading.
func warnAboutTheShell() {
	fmt.Println("  the password was on the command line: it is in this shell's history and was visible in ps")
	fmt.Println("  to drop the history entry: history -d $(history 1 | awk '{print $1}')   # bash/zsh")
}

var _ Seeder = UserSeeder{}
