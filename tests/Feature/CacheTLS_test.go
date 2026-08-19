package feature_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/arandu/bootstrap"
)

// Encryption is asked for by the scheme of the URL and by nothing else, and the
// certificates a self-hosted server needs are file paths the process reads once,
// where the client is built.
//
// Both halves fail quietly if they are wrong -- a connection that was meant to
// be encrypted and is not looks exactly like one that was not, and a certificate
// that was not loaded is a server that trusts anybody. So both are checked
// against what reaches the wire, not against what the configuration says.

// TestTheSchemeIsWhatTurnsEncryptionOn.
//
// A TLS connection opens with a handshake record, byte 0x16; a RESP connection
// opens with the first command, which begins with an asterisk. Reading the first
// byte the application put on the wire is the only check that cannot pass while
// the traffic is in the clear.
func TestTheSchemeIsWhatTurnsEncryptionOn(t *testing.T) {
	for _, c := range []struct {
		name   string
		scheme string
		first  byte
	}{
		{"rediss opens with a handshake", "rediss", 0x16},
		{"redis opens with a command", "redis", '*'},
	} {
		t.Run(c.name, func(t *testing.T) {
			if first := firstByteOnTheWire(t, c.scheme); first != c.first {
				t.Errorf("the connection opened with %#x, want %#x: %s:// put the wrong thing on the wire",
					first, c.first, c.scheme)
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

	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", "rediss://127.0.0.1:1")
	t.Setenv("REDIS_CA_FILE", certFile)
	t.Setenv("REDIS_CERT_FILE", certFile)
	t.Setenv("REDIS_KEY_FILE", keyFile)
	t.Setenv("REDIS_TLS_SERVER_NAME", "cache.example.test")

	if err := bootstrap.Dispatch("routes", nil); err != nil {
		t.Fatalf("the application refused to start with certificates it can read: %v", err)
	}
}

// TestACertificateThatCannotBeUsedStopsTheBoot.
//
// The alternative to refusing is a process that starts with encryption off, or
// without the client certificate the server is going to ask for, after being
// told to use both. Neither is visible from outside, which is why the refusal
// has to name the variable.
func TestACertificateThatCannotBeUsedStopsTheBoot(t *testing.T) {
	certFile, keyFile := writeCertificate(t)
	absent := filepath.Join(t.TempDir(), "absent.pem")

	notPEM := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(notPEM, []byte("this is not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name   string
		url    string
		files  map[string]string
		names  string
		reason string
	}{
		{
			name:   "an authority that is not there",
			url:    "rediss://127.0.0.1:1",
			files:  map[string]string{"REDIS_CA_FILE": absent},
			names:  "REDIS_CA_FILE",
			reason: "the connection cannot verify the server without it",
		},
		{
			name:   "an authority that is not a certificate",
			url:    "rediss://127.0.0.1:1",
			files:  map[string]string{"REDIS_CA_FILE": notPEM},
			names:  "REDIS_CA_FILE",
			reason: "a file that holds no certificate trusts nobody",
		},
		{
			name:   "a certificate without its key",
			url:    "rediss://127.0.0.1:1",
			files:  map[string]string{"REDIS_CERT_FILE": certFile},
			names:  "REDIS_KEY_FILE",
			reason: "half a pair proves nothing",
		},
		{
			name:   "a key without its certificate",
			url:    "rediss://127.0.0.1:1",
			files:  map[string]string{"REDIS_KEY_FILE": keyFile},
			names:  "REDIS_CERT_FILE",
			reason: "half a pair is sent to nobody",
		},
		{
			name:   "a pair that does not go together",
			url:    "rediss://127.0.0.1:1",
			files:  map[string]string{"REDIS_CERT_FILE": certFile, "REDIS_KEY_FILE": notPEM},
			names:  "REDIS_KEY_FILE",
			reason: "a key the certificate does not match authenticates nobody",
		},
		{
			// The one that is not an unreadable file: the certificates are
			// fine and the URL carries no encryption to use them with.
			name:   "certificates for a connection that carries none",
			url:    "redis://127.0.0.1:1",
			files:  map[string]string{"REDIS_CA_FILE": certFile},
			names:  "rediss",
			reason: "believing the traffic is encrypted while it is not is worse than knowing it is not",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			sqliteEnv(t)
			t.Setenv("CACHE_STORE", "redis")
			t.Setenv("REDIS_URL", c.url)
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
		})
	}
}

// firstByteOnTheWire boots the application against a listener of this test's
// own and answers the first byte the connection carried.
//
// The listener closes as soon as it has read that byte, so the client fails at
// once instead of waiting out its read timeout.
func firstByteOnTheWire(t *testing.T, scheme string) byte {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	first := make(chan byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		var opening [1]byte
		if _, err := io.ReadFull(conn, opening[:]); err != nil {
			return
		}
		first <- opening[0]
	}()

	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", scheme+"://"+listener.Addr().String())

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, db, _ := openForTest(t)
	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })

	// The health check is what dials: it asks every module whether it is
	// reachable, and the key-value module answers by pinging.
	rec := httptest.NewRecorder()
	app.Kernel.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))

	select {
	case opening := <-first:
		return opening
	case <-time.After(10 * time.Second):
		t.Fatalf("nothing connected to the listener; the health check answered %d", rec.Code)
		return 0
	}
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
