package nativeauth_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	requests "github.com/arandu-io/arandu/app/Http/Requests"
)

func TestAuthenticationRequestsRedactPasswordsFromJSONAndLogs(t *testing.T) {
	tests := []struct {
		name        string
		request     any
		secrets     []string
		markerCount int
		safeValues  []string
	}{
		{
			name: "login", request: requests.LoginRequest{
				Email: "reader@example.test", Password: "login-secret-9ff2", Remember: true,
			},
			secrets: []string{"login-secret-9ff2"}, markerCount: 1,
			safeValues: []string{"reader@example.test"},
		},
		{
			name: "registration", request: requests.RegisterRequest{
				Name: "Test Reader", Email: "new-reader@example.test",
				Password: "registration-secret-e143", PasswordConfirmation: "confirmation-secret-42ad",
			},
			secrets: []string{"registration-secret-e143", "confirmation-secret-42ad"}, markerCount: 2,
			safeValues: []string{"Test Reader", "new-reader@example.test"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.request)
			if err != nil {
				t.Fatalf("marshalling the request: %v", err)
			}

			var log bytes.Buffer
			slog.New(slog.NewTextHandler(&log, nil)).Info("request", "input", test.request)
			outputs := []struct {
				name string
				text string
			}{
				{name: "json", text: string(encoded)},
				{name: "structured log", text: log.String()},
			}
			for _, output := range outputs {
				for _, secret := range test.secrets {
					if strings.Contains(output.text, secret) {
						t.Errorf("%s exposed %q: %s", output.name, secret, output.text)
					}
				}
				for _, safe := range test.safeValues {
					if !strings.Contains(output.text, safe) {
						t.Errorf("%s omitted safe value %q: %s", output.name, safe, output.text)
					}
				}
				if got := strings.Count(output.text, "[redacted]"); got != test.markerCount {
					t.Errorf("%s contains %d redaction markers, want %d: %s", output.name, got, test.markerCount, output.text)
				}
			}
		})
	}
}
