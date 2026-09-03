package unit_test

import (
	"testing"
)

// TestTheSessionCookieIsSecureUnlessTheEnvironmentNamesDevelopment.
//
// The attribute was derived from the parsed environment, and that value cannot
// answer the question the derivation asks. APP_ENV is parsed into a typed
// environment whose default is development, so by the time it is a value,
// "nobody wrote the variable" and "somebody wrote development" are the same
// word -- and the derivation dropped Secure for both.
//
// The first of the two is an ordinary production deployment. TLS ends at the
// proxy, the process listens on http inside the network, and APP_URL carries
// that internal address; nothing names https and nothing names an environment.
// That is the combination the cookie went out in the clear under, on every
// request, with nothing said at boot.
//
// So the default is inverted: the attribute is present unless APP_ENV was
// written and names development. What still removes it is SESSION_SECURE, the
// variable somebody has to type -- development over http needs it off, because
// a Secure cookie never reaches a browser on http://localhost, and that case
// names itself.
func TestTheSessionCookieIsSecureUnlessTheEnvironmentNamesDevelopment(t *testing.T) {
	for _, c := range []struct {
		name   string
		appEnv string
		appURL string
		secure string
		want   bool
	}{
		{
			name: "an environment that names nothing",
			want: true,
		},
		{
			name:   "an http address and no environment",
			appURL: "http://app.internal:8080",
			want:   true,
		},
		{
			name:   "an https address and no environment",
			appURL: "https://billing.example.test",
			want:   true,
		},
		{
			name:   "development, named",
			appEnv: "dev",
			appURL: "http://localhost:8080",
			want:   false,
		},
		{
			name:   "staging, named",
			appEnv: "staging",
			appURL: "http://app.internal:8080",
			want:   true,
		},
		{
			name:   "production, named",
			appEnv: "prod",
			appURL: "http://app.internal:8080",
			want:   true,
		},
		{
			name:   "SESSION_SECURE removes it outside development",
			appEnv: "prod",
			appURL: "https://billing.example.test",
			secure: "false",
			want:   false,
		},
		{
			name:   "SESSION_SECURE restores it inside development",
			appEnv: "dev",
			appURL: "https://localhost:8443",
			secure: "true",
			want:   true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := loadConfigurationWith(t, map[string]string{
				"APP_ENV":        c.appEnv,
				"APP_URL":        c.appURL,
				"SESSION_SECURE": c.secure,
				// APP_DEBUG follows APP_ENV when nothing writes it, and one
				// left in the shell refuses the named-production case at
				// Validate -- an error about a variable no case here sets,
				// instead of the answer being asked for.
				"APP_DEBUG": "",
			})
			if err != nil {
				t.Fatalf("loading the configuration: %v", err)
			}
			if cfg.Session.Secure != c.want {
				t.Errorf("Session.Secure = %t, want %t (APP_ENV=%q APP_URL=%q SESSION_SECURE=%q)",
					cfg.Session.Secure, c.want, c.appEnv, c.appURL, c.secure)
			}
		})
	}
}
