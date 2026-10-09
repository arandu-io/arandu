// Example resource. Remove with the list under "The example resource" in README.md.

package unit_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/http/client"

	clients "github.com/arandu-io/arandu/app/Clients"
)

// TestTheNewsletterFakeStandsInForTheClient is the shape every test of code that
// calls Newsletter takes: the code depends on clients.Newsletter, and the
// test hands it the fake and reads what was asked.
func TestTheNewsletterFakeStandsInForTheClient(t *testing.T) {
	fake := &clients.NewsletterFake{StatusAnswer: clients.NewsletterStatus{OK: true}}
	var vendor clients.Newsletter = fake

	status, err := vendor.Status(context.Background())
	if err != nil || !status.OK {
		t.Fatalf("Status = %+v, %v: want the answer the fake was given", status, err)
	}
	if len(fake.Calls) != 1 || fake.Calls[0] != "Status" {
		t.Errorf("the fake recorded %v, want one Status", fake.Calls)
	}
}

// TestTheNewsletterClientSendsWhatItIsConfiguredWith runs the real client against a
// faked factory: the request it builds is read, and nothing leaves the
// process.
func TestTheNewsletterClientSendsWhatItIsConfiguredWith(t *testing.T) {
	var sent *http.Request
	factory := client.NewFactory(nil).Fake(func(r *http.Request) (*http.Response, error) {
		sent = r
		return client.NewResponseFromBytes(http.StatusOK, []byte(`{"ok":true}`), nil).HTTPResponse(), nil
	})
	c := clients.NewNewsletterClient(clients.NewsletterConfig{BaseURL: "https://newsletter.example.test", Token: "secret"}, factory)

	status, err := c.Status(context.Background())
	if err != nil || !status.OK {
		t.Fatalf("Status = %+v, %v", status, err)
	}
	if sent == nil || sent.URL.Path != "/status" || sent.Header.Get("Authorization") != "Bearer secret" {
		t.Errorf("the request was not the one configured: %+v", sent)
	}
}

// arandu:begin custom

// TestTheNewsletterClientPostsTheDigestOnce runs the real client against a
// faked factory: the digest leaves as JSON, with the key that makes a retry
// the same request, and nothing leaves the process.
func TestTheNewsletterClientPostsTheDigestOnce(t *testing.T) {
	var sent *http.Request
	var body string
	factory := client.NewFactory(nil).Fake(func(r *http.Request) (*http.Response, error) {
		sent = r
		if r.Body != nil {
			read, _ := io.ReadAll(r.Body)
			body = string(read)
		}
		return client.NewResponseFromBytes(http.StatusAccepted, nil, nil).HTTPResponse(), nil
	})
	c := clients.NewNewsletterClient(clients.NewsletterConfig{BaseURL: "https://newsletter.example.test", Token: "secret"}, factory)

	err := c.SendDigest(context.Background(), clients.NewsletterDigest{
		Key:   "job-1",
		Notes: []clients.NewsletterNote{{Title: "Groceries"}},
	})
	if err != nil {
		t.Fatalf("SendDigest: %v", err)
	}
	if sent == nil || sent.Method != http.MethodPost || sent.URL.Path != "/digests" ||
		sent.Header.Get("Idempotency-Key") != "job-1" || !strings.Contains(body, `"title":"Groceries"`) {
		t.Errorf("the request was not the digest: %+v\n%s", sent, body)
	}
}

// arandu:end custom
