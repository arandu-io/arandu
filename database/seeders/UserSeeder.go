package seeders

import (
	"context"
	"errors"
	"fmt"
	"strings"

	services "github.com/arandu-io/arandu/app/Services"
)

// UserSeeder creates one account a person signs in as, or replaces its
// password.
//
//	aru db:seed UserSeeder -e you@example.com -p a-long-password [-n Name] [-r admin]
//	aru db:seed UserSeeder -upd -e you@example.com -p a-new-password
//
// It is the operator's door: the first administrator of a fresh deployment, or
// somebody locked out before mail is configured, has no reset link to follow.
// It goes through the user service rather than the factory, because this
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
		return errors.New("the user service and the tenant must be wired: an account in no tenant is one nobody can sign in as")
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
	case errors.Is(err, services.ErrUserNotFound):
		user, err := d.Users.EnsureUser(ctx, d.Tenant, name, email, password, roles, true)
		if err != nil {
			return err
		}
		fmt.Printf("created %s%s\n", user.Email, describeRoles(user.Roles))
		return nil
	case err != nil:
		return fmt.Errorf("looking the address up: %w", err)
	case !Switch(d.Args, "upd") && !Switch(d.Args, "update"):
		fmt.Printf("%s already exists and was left as it is%s\n", existing.Email, describeRoles(existing.Roles))
		fmt.Printf("  to replace the password: aru db:seed UserSeeder -upd -e %s -p <password>\n", existing.Email)
		return nil
	}

	updated, err := d.Users.SetPassword(ctx, d.Tenant, email, password)
	if err != nil {
		return fmt.Errorf("replacing the password: %w", err)
	}
	if len(roles) > 0 {
		fmt.Println("  -r was ignored: this replaces the password and nothing else")
	}
	fmt.Printf("password replaced for %s%s\n", updated.Email, describeRoles(updated.Roles))
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

func describeRoles(roles []string) string {
	if len(roles) == 0 {
		return ", with no roles"
	}
	return ", with " + strings.Join(roles, " and ")
}

var _ Seeder = UserSeeder{}
