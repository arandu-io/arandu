package config

import framework "github.com/arandu-io/framework/config"

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
	// MailerSES is Amazon SES, through the mail/ses submodule.
	MailerSES Mailer = "ses"
)

// Mail is what config/mail.php holds.
type Mail struct {
	Mailer Mailer

	// Host, Port, Username and Password describe the SMTP server.
	Host     string
	Port     int
	Username string
	Password string

	// Encryption is tls or starttls. There is no "none": an unencrypted SMTP
	// session sends the password in the clear.
	Encryption string

	// FromAddress and FromName are the envelope every message leaves with.
	FromAddress string
	FromName    string
}

func loadMail(base framework.Config) Mail {
	return Mail{
		Mailer:      Mailer(env("MAIL_MAILER", string(MailerLog))),
		Host:        env("MAIL_HOST", "127.0.0.1"),
		Port:        envInt("MAIL_PORT", 587),
		Username:    env("MAIL_USERNAME", ""),
		Password:    env("MAIL_PASSWORD", ""),
		Encryption:  env("MAIL_ENCRYPTION", "starttls"),
		FromAddress: env("MAIL_FROM_ADDRESS", "no-reply@localhost"),
		FromName:    env("MAIL_FROM_NAME", base.AppName),
	}
}
