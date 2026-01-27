package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"golang.org/x/crypto/ssh"
)

func Test_connectSSH(t *testing.T) {
	orig := sshDial
	defer func() { sshDial = orig }()

	sshDial = func(network, addr string, cfg *ssh.ClientConfig) (*ssh.Client, error) {
		if addr != "example.com:22" {
			t.Fatalf("unexpected addr: %s", addr)
		}
		if cfg.User != "test_user" {
			t.Fatalf("unexpected user: %s", cfg.User)
		}
		return &ssh.Client{}, nil
	}

	_, err := Connect("test_user", "example.com", generateTestEd25519Key(t, "PRIVATE KEY"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = Connect("test_user", "example.com", generateTestEd25519Key(t, "RSA PRIVATE KEY"))
	if err == nil {
		t.Fatalf("expected an error parsing the key")
	}
}

func generateTestEd25519Key(t testing.TB, pemType string) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}

	var buf bytes.Buffer
	err = pem.Encode(&buf, &pem.Block{
		Type:  pemType,
		Bytes: privDER,
	})
	if err != nil {
		t.Fatalf("pem encode: %v", err)
	}

	return buf.String()
}
