package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/arandu-io/framework/foundation/bootstrap"
)

// Mailer is the transport outgoing mail leaves by.
type Mailer string

// The supported mailers.
const (
	// MailerLog writes the message to the log instead of sending it. The
	// development default: a framework that sends real mail from a laptop is a
	// framework that mails a customer during a test.
	MailerLog Mailer = "log"
	// MailerSMTP is any SMTP server.
	MailerSMTP Mailer = "smtp"
	// MailerResend is Resend, and it is what this example is configured for.
	//
	// Resend and SendGrid rather than Amazon SES, and it is a decision rather
	// than an omission: SES needs an AWS account, a sending identity, a region
	// and a sandbox exit request before the first message leaves, and these two
	// need an API key.
	//
	// Both are in the core rather than in a submodule, because neither needs a
	// dependency to reach: one POST with a JSON body is net/http, which every
	// binary already links.
	MailerResend Mailer = "resend"
	// MailerSendGrid is SendGrid.
	MailerSendGrid Mailer = "sendgrid"
	// MailerArray keeps messages in memory, for tests to read.
	MailerArray Mailer = "array"
)

// Mail is where outgoing messages go, and it comes from one URL.
//
//	MAIL_URL=log://                                   the development default
//	MAIL_URL=smtp://user:password@smtp.example.com:587
//	MAIL_URL=resend://re_xxxxxxxx
//	MAIL_URL=sendgrid://SG.xxxxxxxx
//	MAIL_URL=array://                                 what tests read
//
// One URL rather than seven variables, for the same reason the database has
// one: the scheme is the transport, so a configuration cannot say resend and
// carry an SMTP host, and a credential cannot be left behind in a variable the
// new transport does not read.
//
// The sender is not part of it. MAIL_FROM_ADDRESS and MAIL_FROM_NAME are who
// the application is, not where the message goes, and they do not change when
// the provider does.
type Mail struct {
	Mailer Mailer

	// Host, Port, Username and Password describe the SMTP server, parsed out of
	// the URL when the scheme is smtp.
	Host     string
	Port     int
	Username string
	Password string

	// Encryption is tls or starttls. There is no "none": an unencrypted SMTP
	// session sends the password in the clear. smtps:// asks for implicit TLS;
	// smtp:// is STARTTLS, which is what port 587 means.
	Encryption string

	// Key authenticates with Resend or SendGrid, and it is the host of the URL:
	// resend://re_xxxxxxxx.
	Key string

	// FromAddress and FromName are the envelope every message leaves with.
	FromAddress string
	FromName    string
}

// retiredMailVars are the variables MAIL_URL replaced. Setting one is refused
// rather than ignored: a MAIL_KEY nobody reads is a provider silently not
// configured, and the first thing anybody notices is a customer who never got
// the mail.
var retiredMailVars = []string{
	"MAIL_MAILER", "MAIL_HOST", "MAIL_PORT",
	"MAIL_USERNAME", "MAIL_PASSWORD", "MAIL_ENCRYPTION", "MAIL_KEY",
}

func loadMail(base bootstrap.Configuration) (Mail, error) {
	for _, name := range retiredMailVars {
		if env(name, "") == "" {
			continue
		}
		return Mail{}, fmt.Errorf("%s is retired; remove it and configure the mailer with MAIL_URL", name)
	}

	cfg, err := parseMailURL(env("MAIL_URL", "log://"))
	if err != nil {
		return Mail{}, err
	}
	cfg.FromAddress = env("MAIL_FROM_ADDRESS", "no-reply@localhost")
	cfg.FromName = env("MAIL_FROM_NAME", base.App.Name)
	return cfg, nil
}

// parseMailURL reads the one variable.
//
// A bad value is returned to the boot boundary rather than falling back to the
// log transport. Falling back would report a successful boot while outgoing
// messages go somewhere the deployment did not choose.
func parseMailURL(raw string) (Mail, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "log://"
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return Mail{}, fmt.Errorf("MAIL_URL has invalid URL value %q", raw)
	}

	cfg := Mail{Mailer: Mailer(strings.ToLower(u.Scheme))}

	switch cfg.Mailer {
	case MailerLog, MailerArray:
		// Nothing to configure. Both are complete in their scheme.

	case MailerSMTP, "smtps":
		cfg.Mailer = MailerSMTP
		cfg.Host = u.Hostname()
		cfg.Port = 587
		if rawPort := u.Port(); rawPort != "" {
			port, err := strconv.Atoi(rawPort)
			if err != nil || port <= 0 {
				return Mail{}, fmt.Errorf("MAIL_URL has invalid SMTP port %q", rawPort)
			}
			cfg.Port = port
		}
		if u.User != nil {
			cfg.Username = u.User.Username()
			cfg.Password, _ = u.User.Password()
		}
		// smtps is implicit TLS on 465; smtp is STARTTLS, which is what 587
		// means. Naming the difference in the scheme is what stops a
		// configuration asking for one port and one handshake.
		cfg.Encryption = "starttls"
		if strings.EqualFold(u.Scheme, "smtps") {
			cfg.Encryption = "tls"
			if u.Port() == "" {
				cfg.Port = 465
			}
		}
		if cfg.Host == "" {
			return Mail{}, fmt.Errorf("MAIL_URL names no SMTP host")
		}

	case MailerResend, MailerSendGrid:
		// The key is the host: resend://re_xxxxxxxx. There is nothing else to
		// say to either provider, and a key in the userinfo would be a key that
		// looks like a username with no password.
		cfg.Key = u.Host
		if cfg.Key == "" {
			return Mail{}, fmt.Errorf("MAIL_URL carries no API key for %s", u.Scheme)
		}

	default:
		return Mail{}, fmt.Errorf("MAIL_URL uses unsupported scheme %q; expected log, smtp, smtps, resend, sendgrid or array", u.Scheme)
	}

	return cfg, nil
}
