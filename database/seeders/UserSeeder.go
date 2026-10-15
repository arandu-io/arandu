package seeders

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/arandu-io/hesape/auth/account"

	models "github.com/arandu-io/arandu/app/Models"
)

// UserSeeder creates one account a person signs in as, or replaces its
// password.
//
//	aru db:seed UserSeeder -e you@example.com -p a-long-password [-n Name] [-r admin]
//	aru db:seed UserSeeder -upd -e you@example.com -p a-new-password
//
// It is the operator's door: the first administrator of a fresh deployment, or
// somebody locked out before mail is configured, has no reset link to follow.
// It goes through the account rules rather than the factory, because this
// account is real: its address is normalised, its password hashed, and a
// duplicate refused by the same rules as a registration.
//
// The account is created verified, and an address that already exists is left
// alone unless -upd says to replace its password -- and then only its password:
// -r is ignored, so a password reset never makes an administrator by accident.
//
// The password is on the command line, so it lands in the shell history and in
// ps while this runs. The command says so every time, and the remedy is to
// change it from the application once signed in.
type UserSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (UserSeeder) Name() string { return "UserSeeder" }

// Run creates the account, or replaces its password when -upd is given.
func (UserSeeder) Run(ctx context.Context, d Deps) error {
	if d.Users == nil || d.Tenant == "" {
		return errors.New("the account rules and the tenant must be wired: an account in no tenant is one nobody can sign in as")
	}
	email, password, name := flag(d.Args, "e", "email"), flag(d.Args, "p", "password"), flag(d.Args, "n", "name")
	if email == "" || password == "" {
		return errors.New("-e and -p are both required:\n\n" +
			"    aru db:seed UserSeeder -e you@example.com -p a-long-password [-r admin]\n" +
			"    aru db:seed UserSeeder -upd -e you@example.com -p a-new-password")
	}
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	roles := rolesFrom(d.Args)

	fmt.Println("  the password was on the command line: it is in this shell's history and was visible in ps")
	fmt.Println("  to drop the history entry: history -d $(history 1 | awk '{print $1}')   # bash/zsh")

	existing, err := d.Users.Lookup(ctx, d.Tenant, email)
	switch {
	case errors.Is(err, account.ErrUserNotFound):
		user, err := d.Users.Provision(ctx, d.Tenant, account.Draft{
			Name: name, Email: email, Password: password, Verified: true,
			Fill: func(a account.Account) error { return withRoles(a, roles) },
		})
		if err != nil {
			return err
		}
		fmt.Printf("created %s%s\n", user.AccountEmail(), describeRoles(user))
		return nil
	case err != nil:
		return fmt.Errorf("looking the address up: %w", err)
	case !Switch(d.Args, "upd") && !Switch(d.Args, "update"):
		fmt.Printf("%s already exists and was left as it is%s\n", existing.AccountEmail(), describeRoles(existing))
		fmt.Printf("  to replace the password: aru db:seed UserSeeder -upd -e %s -p <password>\n", existing.AccountEmail())
		return nil
	}

	updated, err := d.Users.SetPassword(ctx, d.Tenant, email, password)
	if err != nil {
		return fmt.Errorf("replacing the password: %w", err)
	}
	if len(roles) > 0 {
		fmt.Println("  -r was ignored: this replaces the password and nothing else")
	}
	fmt.Printf("password replaced for %s%s\n", updated.AccountEmail(), describeRoles(updated))
	return nil
}

// flag reads the first of the spellings that is present.
func flag(args []string, short, long string) string {
	if value, _ := Flag(args, short); value != "" {
		return value
	}
	value, _ := Flag(args, long)
	return value
}

// rolesFrom collects every -r, so an account can hold more than one. Flag
// answers the first match, which is right for an address and wrong for a list.
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

// withRoles writes the roles of a new account before it is stored. The account
// rules build this application's *models.User, and its roles are a column the
// rules know nothing about.
func withRoles(a account.Account, roles []string) error {
	user, ok := a.(*models.User)
	if !ok {
		return fmt.Errorf("the account rules built a %T, not a *models.User", a)
	}
	user.Roles = append(models.Roles(nil), roles...)
	return nil
}

// describeRoles names the roles an account holds, for the line printed about
// it.
func describeRoles(a account.Account) string {
	var roles []string
	if user, ok := a.(*models.User); ok {
		roles = user.Roles
	}
	if len(roles) == 0 {
		return ", with no roles"
	}
	return ", with " + strings.Join(roles, " and ")
}

var _ Seeder = UserSeeder{}
