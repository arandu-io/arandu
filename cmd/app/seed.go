package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"github.com/arandu-io/framework/modules/auth"
)

// seedAdmin creates the first administrator of a tenant.
//
// It exists because there is no other way in: every repository call needs a
// Grant, and a Grant needs a subject, so the very first user cannot be created
// through the application itself. This is the one command that uses a system
// grant to break that circle, and it does it on purpose, in the open.
//
// Credentials come from the environment rather than from flags: a password typed
// as an argument lands in the shell history and in the process list.
func seedAdmin(ctx context.Context, svc *auth.Service) error {
	email := os.Getenv("ARANDU_ADMIN_EMAIL")
	password := os.Getenv("ARANDU_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return errors.New("set ARANDU_ADMIN_EMAIL and ARANDU_ADMIN_PASSWORD before running seed:admin")
	}

	tenant := os.Getenv("ARANDU_TENANT_ID")
	generated := false
	if tenant == "" {
		id, err := newUUID()
		if err != nil {
			return err
		}
		tenant, generated = id, true
	}

	user, err := svc.EnsureAdmin(ctx, tenant, email, password)
	if err != nil {
		return err
	}

	fmt.Printf("admin %s ready in tenant %s\n", user.Email, user.TenantID)
	if generated {
		fmt.Printf("\nthis tenant was generated. Put it in .env so the login form can find it:\n  ARANDU_TENANT_ID=%s\n", user.TenantID)
	}
	return nil
}

// newUUID builds a version 4 UUID from crypto/rand.
//
// Sixteen lines instead of a dependency: the database generates the ids of every
// row through gen_random_uuid, and this is the only place the application itself
// needs to mint one.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
