// Example resource. Remove with the list under "The example resource" in README.md.

package clients

import "context"

// NewsletterFake is Newsletter for the tests: it answers what it was told to,
// records what it was asked, and never reaches the network. A service under
// test is built with one in place of NewsletterClient.
type NewsletterFake struct {
	// StatusAnswer is what Status answers.
	StatusAnswer NewsletterStatus
	// Err, when set, is what every call answers instead.
	Err error
	// Calls are the methods called, in order.
	Calls []string
	// Digests are the digests SendDigest was handed and accepted, in order.
	Digests []NewsletterDigest
}

// Compile-time proof that the fake stands in for the client.
var _ Newsletter = (*NewsletterFake)(nil)

// Status records the call and answers StatusAnswer, or Err.
func (f *NewsletterFake) Status(ctx context.Context) (NewsletterStatus, error) {
	f.Calls = append(f.Calls, "Status")
	if f.Err != nil {
		return NewsletterStatus{}, f.Err
	}
	return f.StatusAnswer, nil
}

// arandu:begin custom

// SendDigest records the call and the digest, and answers Err.
func (f *NewsletterFake) SendDigest(ctx context.Context, digest NewsletterDigest) error {
	f.Calls = append(f.Calls, "SendDigest")
	if f.Err != nil {
		return f.Err
	}
	f.Digests = append(f.Digests, digest)
	return nil
}

// arandu:end custom
