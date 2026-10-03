package feature_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/hesape/cache"

	"github.com/arandu-io/arandu/bootstrap"
)

// Encryption is asked for by the scheme of the URL and by nothing else, and the
// certificates a self-hosted server needs are file paths the process reads once,
// before the connector is asked for a store.
//
// Both halves fail quietly if they are wrong -- a connection that was meant to
// be encrypted and is not looks exactly like one that was not, and a certificate
// that was not loaded is a server that trusts anybody. So both are checked
// against what the connector is handed, not against what the configuration
// says; and what the connector puts on the wire for what it is handed is
// checked where the connector is, against a listener of its own:
// TestOpenHandsTheEndpointToTheConnection in github.com/arandu-io/hesape/redis,
// and TestPingSucceedsOverTLSSignedByAPrivateAuthority and
// TestPingFailsWithoutTLSAgainstAServerThatSpeaksIt in its connections package.

// TestTheSchemeIsWhatTurnsEncryptionOn.
//
// rediss:// hands the connector a TLS configuration, with the floor this
// application sets, and redis:// hands it none. Reading the endpoint the
// connector was opened over is the only check that cannot pass while the
// configuration says one thing and the store is opened with another.
func TestTheSchemeIsWhatTurnsEncryptionOn(t *testing.T) {
	for _, c := range []struct {
		name      string
		scheme    string
		encrypted bool
	}{
		{"rediss hands over a TLS configuration", "rediss", true},
		{"redis hands over none", "redis", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := endpointOpenedFor(t, c.scheme)
			if (e.TLS != nil) != c.encrypted {
				t.Fatalf("%s:// opened the store with TLS %v, want encryption %t", c.scheme, e.TLS, c.encrypted)
			}
			if c.encrypted && e.TLS.MinVersion != tls.VersionTLS12 {
				t.Errorf("the TLS floor is %#x, want TLS 1.2: a connection carrying the password and every session id "+
					"is not where to accept an older version", e.TLS.MinVersion)
			}
		})
	}
}

// TestTheNamedCertificatesAreLoaded: a certificate the deployment named and the
// process never read is a private authority that is not trusted and a client
// certificate that is never sent, and the connection fails later for a reason
// that names neither.
func TestTheNamedCertificatesAreLoaded(t *testing.T) {
	certFile, keyFile := writeCertificate(t)

	// The real connector, when the project links one, dials nothing at the
	// boot, so a port nothing listens on is enough for it; the fake keeps a
	// record of what it was handed, which is what is read below.
	address, server := "127.0.0.1:1", (*fakeRESP)(nil)
	if linkRESP() {
		server = startRESP(t)
		address = server.address
	}

	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", "rediss://"+address)
	t.Setenv("REDIS_CA_FILE", certFile)
	t.Setenv("REDIS_CERT_FILE", certFile)
	t.Setenv("REDIS_KEY_FILE", keyFile)
	t.Setenv("REDIS_TLS_SERVER_NAME", "cache.example.test")

	if err := bootstrap.Dispatch("routes", nil); err != nil {
		t.Fatalf("the application refused to start with certificates it can read: %v", err)
	}
	if server == nil {
		return
	}

	opened := server.opened()
	if len(opened) == 0 {
		t.Fatal("the store was never opened, so nothing below was handed to anybody")
	}
	encryption := opened[len(opened)-1].TLS
	if encryption == nil {
		t.Fatal("rediss:// and three certificate files, and the store was opened in the clear")
	}
	if encryption.RootCAs == nil {
		t.Error("REDIS_CA_FILE was named and the connector was handed no private authority")
	}
	if len(encryption.Certificates) != 1 {
		t.Errorf("REDIS_CERT_FILE and REDIS_KEY_FILE were named and the connector was handed %d client certificates, want 1",
			len(encryption.Certificates))
	}
	if encryption.ServerName != "cache.example.test" {
		t.Errorf("the server name is %q, want what REDIS_TLS_SERVER_NAME says", encryption.ServerName)
	}
}

// TestACertificateThatCannotBeUsedStopsTheBoot.
//
// The alternative to refusing is a process that starts with encryption off, or
// without the client certificate the server is going to ask for, after being
// told to use both. Neither is visible from outside, which is why the refusal
// has to name the variable -- and why it comes before the connector is asked
// for anything.
func TestACertificateThatCannotBeUsedStopsTheBoot(t *testing.T) {
	certFile, keyFile := writeCertificate(t)
	absent := filepath.Join(t.TempDir(), "absent.pem")

	notPEM := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(notPEM, []byte("this is not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name   string
		scheme string
		files  map[string]string
		names  string
		reason string
	}{
		{
			name:   "an authority that is not there",
			scheme: "rediss",
			files:  map[string]string{"REDIS_CA_FILE": absent},
			names:  "REDIS_CA_FILE",
			reason: "the connection cannot verify the server without it",
		},
		{
			name:   "an authority that is not a certificate",
			scheme: "rediss",
			files:  map[string]string{"REDIS_CA_FILE": notPEM},
			names:  "REDIS_CA_FILE",
			reason: "a file that holds no certificate trusts nobody",
		},
		{
			name:   "a certificate without its key",
			scheme: "rediss",
			files:  map[string]string{"REDIS_CERT_FILE": certFile},
			names:  "REDIS_KEY_FILE",
			reason: "half a pair proves nothing",
		},
		{
			name:   "a key without its certificate",
			scheme: "rediss",
			files:  map[string]string{"REDIS_KEY_FILE": keyFile},
			names:  "REDIS_CERT_FILE",
			reason: "half a pair is sent to nobody",
		},
		{
			name:   "a pair that does not go together",
			scheme: "rediss",
			files:  map[string]string{"REDIS_CERT_FILE": certFile, "REDIS_KEY_FILE": notPEM},
			names:  "REDIS_KEY_FILE",
			reason: "a key the certificate does not match authenticates nobody",
		},
		{
			// The one that is not an unreadable file: the certificates are
			// fine and the URL carries no encryption to use them with.
			name:   "certificates for a connection that carries none",
			scheme: "redis",
			files:  map[string]string{"REDIS_CA_FILE": certFile},
			names:  "rediss",
			reason: "believing the traffic is encrypted while it is not is worse than knowing it is not",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			address, server := unansweredRESP(t)

			sqliteEnv(t)
			t.Setenv("CACHE_STORE", "redis")
			t.Setenv("REDIS_URL", c.scheme+"://"+address)
			for name, path := range c.files {
				t.Setenv(name, path)
			}

			err := bootstrap.Dispatch("routes", nil)
			if err == nil {
				t.Fatalf("the application started anyway, and %s", c.reason)
			}
			if !strings.Contains(err.Error(), c.names) {
				t.Errorf("the refusal does not name %s: %v", c.names, err)
			}
			if server != nil && len(server.opened()) != 0 {
				t.Error("the connector was asked for a store before the files it would be handed were read")
			}
		})
	}
}

// endpointOpenedFor boots the application with the shared store at a server of
// the fake connector's, reached by scheme, and answers the endpoint the store
// was opened over.
func endpointOpenedFor(t *testing.T, scheme string) cache.Endpoint {
	t.Helper()

	server := borrowRESP(t)
	sqliteEnv(t)

	// The schema is built before the store is named, for the reason the health
	// tests build it that way: this test is about what the store is opened
	// over, and needs a migrated database rather than a migration.
	migrateWithoutTheStore(t)

	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", scheme+"://"+server.address)

	// Building the application is what opens the store: the rate limit
	// resolves the store CACHE_STORE named while the pipeline is assembled.
	cfg, db, _ := openForTest(t)
	if _, err := bootstrap.Build(cfg, db); err != nil {
		t.Fatalf("Build: %v", err)
	}

	opened := server.opened()
	if len(opened) == 0 {
		t.Fatal("the application was built with CACHE_STORE=redis and never opened the store")
	}
	return opened[len(opened)-1]
}

// writeCertificate writes a self-signed certificate and its key, and answers the
// two paths.
//
// One certificate serves as the private authority and as the client's own,
// because what is being checked is that named files are read and parsed -- not
// that a real chain validates, which is the server's half and not this
// application's.
func writeCertificate(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cache.example.test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"cache.example.test"},
	}
	body, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating a certificate: %v", err)
	}
	encoded, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("encoding the key: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	write := func(path, kind string, der []byte) {
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	write(certFile, "CERTIFICATE", body)
	write(keyFile, "EC PRIVATE KEY", encoded)

	return certFile, keyFile
}
