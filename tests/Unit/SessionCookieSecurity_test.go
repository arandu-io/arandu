package unit_test

import (
	"strings"
	"testing"
)

// TestTheSessionCookieIsSecureUnlessTheEnvironmentIsDev reads the one decision
// of the Secure attribute through the configuration a boot loads.
//
// The framework's loader is its only reader: SESSION_SECURE_COOKIE when it is
// written, and otherwise Secure in every environment except dev. This project
// reads no variable of its own for it any more -- the session store, the CSRF
// guest cookie and the flash cookie all take Config.Framework.Session.Secure --
// so what is asserted is that value, under the three ways a deployment can
// answer: by naming dev, by naming anything else, and by declaring the variable.
//
// APP_URL takes no part. Behind a proxy that ends TLS the address this process
// knows is http, and the browser's connection is https all the same.
func TestTheSessionCookieIsSecureUnlessTheEnvironmentIsDev(t *testing.T) {
	for _, c := range []struct {
		name   string
		appEnv string
		appURL string
		secure string
		want   bool
	}{
		{name: "dev, named", appEnv: "dev", appURL: "http://localhost:8080", want: false},
		{name: "dev, with an https address", appEnv: "dev", appURL: "https://localhost:8443", want: false},
		{name: "staging over http, nothing declared", appEnv: "staging", appURL: "http://app.internal:8080", want: true},
		{name: "prod over http, nothing declared", appEnv: "prod", appURL: "http://app.internal:8080", want: true},
		{name: "prod, declared false to serve over http", appEnv: "prod", appURL: "http://app.example.test", secure: "false", want: false},
		{name: "dev, declared true", appEnv: "dev", appURL: "https://localhost:8443", secure: "true", want: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := loadConfigurationWith(t, map[string]string{
				"APP_ENV":               c.appEnv,
				"APP_URL":               c.appURL,
				"SESSION_SECURE_COOKIE": c.secure,
				// APP_DEBUG follows APP_ENV when nothing writes it, and one
				// left in the shell refuses the named-production case at
				// Validate -- an error about a variable no case here sets,
				// instead of the answer being asked for.
				"APP_DEBUG": "",
			})
			if err != nil {
				t.Fatalf("loading the configuration: %v", err)
			}
			if got := cfg.Framework.Session.Secure; got != c.want {
				t.Errorf("Framework.Session.Secure = %t, want %t (APP_ENV=%q APP_URL=%q SESSION_SECURE_COOKIE=%q)",
					got, c.want, c.appEnv, c.appURL, c.secure)
			}
		})
	}
}

// TestTheRetiredSessionVariableIsRefusedAtBoot: SESSION_SECURE was read by
// this project, and nothing reads it now. A deployment that still writes it is
// told which variable replaced it, rather than having its decision dropped
// without a word. Left empty, as .env.example used to write it, it is no
// decision and is not refused.
func TestTheRetiredSessionVariableIsRefusedAtBoot(t *testing.T) {
	_, err := loadConfigurationWith(t, map[string]string{"SESSION_SECURE": "false"})
	if err == nil {
		t.Fatal("the boot accepted SESSION_SECURE=false, a variable nothing reads")
	}
	for _, want := range []string{"SESSION_SECURE is retired", "SESSION_SECURE_COOKIE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}

	if _, err := loadConfigurationWith(t, map[string]string{"SESSION_SECURE": "", "SESSION_COOKIE": "arandu_session"}); err != nil {
		t.Errorf("the boot refused the two session lines an older .env.example wrote: %v", err)
	}
}

// TestTheCookieScopeVariablesAreRefusedAtBoot: SESSION_PATH, SESSION_DOMAIN and
// SESSION_SAME_SITE were read into the configuration and then taken by nothing,
// because the session store writes its cookie for path /, for the host that
// answered, with SameSite=Lax. A value that asks for another cookie is refused
// naming the variable; the value that states what the store already writes is
// no request, and SESSION_PATH=/ is in every .env copied from an older
// .env.example, so it is accepted.
func TestTheCookieScopeVariablesAreRefusedAtBoot(t *testing.T) {
	for _, c := range []struct {
		name, value string
	}{
		{"SESSION_PATH", "/app"},
		{"SESSION_DOMAIN", "example.test"},
		{"SESSION_DOMAIN", ".example.test"},
		{"SESSION_SAME_SITE", "strict"},
		{"SESSION_SAME_SITE", "none"},
	} {
		t.Run(c.name+"="+c.value, func(t *testing.T) {
			_, err := loadConfigurationWith(t, map[string]string{c.name: c.value})
			if err == nil {
				t.Fatalf("the boot accepted %s=%s, which nothing reads: the cookie is written without it", c.name, c.value)
			}
			for _, want := range []string{c.name + " is retired", "SameSite=Lax"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
		})
	}

	for _, values := range []map[string]string{
		{"SESSION_PATH": "/", "SESSION_DOMAIN": ""},
		{"SESSION_SAME_SITE": "lax"},
		{"SESSION_SAME_SITE": "Lax"},
	} {
		if _, err := loadConfigurationWith(t, values); err != nil {
			t.Errorf("the boot refused %v, which states the cookie the store writes: %v", values, err)
		}
	}
}

// TestAMalformedSecureCookieDecisionStopsTheBoot: the one decision of the
// Secure attribute is a boolean, and a value that is not one -- a typo, a stray
// space -- is refused at boot rather than read as the default. The default
// differs between dev and everywhere else, so falling back on it in silence
// would be a decision nobody wrote.
func TestAMalformedSecureCookieDecisionStopsTheBoot(t *testing.T) {
	for _, value := range []string{"sometimes", " true"} {
		_, err := loadConfigurationWith(t, map[string]string{"SESSION_SECURE_COOKIE": value})
		if err == nil {
			t.Errorf("the boot accepted SESSION_SECURE_COOKIE=%q and fell back on a default", value)
			continue
		}
		if !strings.Contains(err.Error(), "SESSION_SECURE_COOKIE") {
			t.Errorf("error = %q, want it to name SESSION_SECURE_COOKIE", err)
		}
	}
}
