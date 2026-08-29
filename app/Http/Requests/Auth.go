// Package requests holds browser input contracts.
package requests

import (
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
)

// LoginRequest is a password sign-in input.
type LoginRequest struct {
	Email    string
	Password string
	Remember bool
}

// Validate reports field errors without revealing account state.
func (r LoginRequest) Validate() validation.Errors {
	err := validation.Errors{}
	validation.Required(err, "email", r.Email)
	validation.Required(err, "password", r.Password)
	return err
}

// RegisterRequest is a self-registration input and deliberately carries no roles.
type RegisterRequest struct {
	Name                 string
	Email                string
	Password             string
	PasswordConfirmation string
}

// Validate reports the registration field errors.
func (r RegisterRequest) Validate() validation.Errors {
	err := validation.Errors{}
	validation.Required(err, "name", r.Name)
	validation.MaxLen(err, "name", r.Name, 80)
	validation.Required(err, "email", r.Email)
	validation.Email(err, "email", r.Email)
	validation.MaxLen(err, "email", r.Email, 254)
	validation.MinLen(err, "password", r.Password, security.MinPasswordLen)
	validation.MaxLen(err, "password", r.Password, 128)
	validation.Confirmed(err, "password_confirmation", r.Password, r.PasswordConfirmation)
	return err
}

// EmailCodeRequest is a code bound to one email-code purpose and subject.
type EmailCodeRequest struct{ Code string }

// Validate reports whether a code was supplied.
func (r EmailCodeRequest) Validate() validation.Errors {
	err := validation.Errors{}
	validation.Required(err, "code", r.Code)
	return err
}

var (
	_ validation.Validatable = LoginRequest{}
	_ validation.Validatable = RegisterRequest{}
	_ validation.Validatable = EmailCodeRequest{}
)
