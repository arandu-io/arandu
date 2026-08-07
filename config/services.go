package config

// Credential is what a third-party service needs to be called: an endpoint and
// a pair of secrets. One shape for all of them, so adding a service is a field
// and not a struct.
type Credential struct {
	// URL is the base endpoint, when the service has more than one region or
	// runs self-hosted.
	URL string
	// Key is the public half: an account id, a publishable key, a client id.
	Key string
	// Secret is the half that must never be logged, echoed or dumped.
	Secret string
}

// Configured reports whether the credential is filled in. A service that is
// wired but not configured should refuse at boot rather than fail on the first
// call in production.
func (c Credential) Configured() bool { return c.Key != "" && c.Secret != "" }

// Services is what config/services.php holds: the credentials of everything this
// application calls that is not its own database.
//
// One field per service, added as the application grows. Cloudflare comes first
// because it is the suggested default for storage and for the edge.
type Services struct {
	Cloudflare Credential
	Stripe     Credential
	Postmark   Credential
}

func loadServices() Services {
	return Services{
		Cloudflare: Credential{
			URL:    env("CLOUDFLARE_API_URL", "https://api.cloudflare.com/client/v4"),
			Key:    env("CLOUDFLARE_ACCOUNT_ID", ""),
			Secret: env("CLOUDFLARE_API_TOKEN", ""),
		},
		Stripe: Credential{
			URL:    env("STRIPE_API_URL", "https://api.stripe.com"),
			Key:    env("STRIPE_PUBLISHABLE_KEY", ""),
			Secret: env("STRIPE_SECRET_KEY", ""),
		},
		Postmark: Credential{
			URL:    env("POSTMARK_API_URL", "https://api.postmarkapp.com"),
			Key:    env("POSTMARK_MESSAGE_STREAM", "outbound"),
			Secret: env("POSTMARK_SERVER_TOKEN", ""),
		},
	}
}
