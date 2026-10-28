// Package tokens mints the end-user JWTs used by the e2e suite. A fresh RSA
// signing key is created on every run (nothing is ever committed): the public
// half is published as a JWKS for Istio's RequestAuthentication, and a set of
// valid and deliberately broken tokens is written next to it.
package tokens

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// KeyBits is the RSA modulus size of created signing keys.
const KeyBits = 2048

var b64 = base64.RawURLEncoding

// Claims are the registered and custom claims carried by demo tokens.
type Claims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	Audience  string `json:"aud"`
	Role      string `json:"role,omitempty"`
	IssuedAt  int64  `json:"iat"`
	NotBefore int64  `json:"nbf"`
	ExpiresAt int64  `json:"exp"`
}

// JWK is a public RSA key in JSON Web Key form.
type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS is a JSON Web Key Set.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// Signer signs RS256 tokens with one key.
type Signer struct {
	Key *rsa.PrivateKey
	Kid string
}

// NewSigner generates a new RSA key with the given key ID.
func NewSigner(kid string) (*Signer, error) {
	k, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return &Signer{Key: k, Kid: kid}, nil
}

// PublicJWKS returns the JWKS that verifies tokens from s.
func (s *Signer) PublicJWKS() JWKS {
	pub := s.Key.PublicKey
	return JWKS{Keys: []JWK{{
		Kty: "RSA", Use: "sig", Alg: "RS256", Kid: s.Kid,
		N: b64.EncodeToString(pub.N.Bytes()),
		E: b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}

func encodeSegment(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	return b64.EncodeToString(raw), nil
}

// Sign returns a compact RS256 JWS for c.
func (s *Signer) Sign(c Claims) (string, error) {
	h, err := encodeSegment(map[string]string{"alg": "RS256", "typ": "JWT", "kid": s.Kid})
	if err != nil {
		return "", err
	}
	p, err := encodeSegment(c)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(h + "." + p))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.Key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return h + "." + p + "." + b64.EncodeToString(sig), nil
}

// Unsigned returns an "alg: none" token, which a correct verifier must reject.
func Unsigned(c Claims) (string, error) {
	h, err := encodeSegment(map[string]string{"alg": "none", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	p, err := encodeSegment(c)
	if err != nil {
		return "", err
	}
	return h + "." + p + ".", nil
}

// Verify checks an RS256 token against jwks and returns its claims. It is used
// by the unit tests to prove the created material is self-consistent; the
// mesh does its own verification.
func Verify(token string, jwks JWKS, now time.Time) (Claims, error) {
	var c Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, errors.New("malformed token")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &hdr); err != nil {
		return c, err
	}
	if hdr.Alg != "RS256" {
		return c, fmt.Errorf("unsupported alg %q", hdr.Alg)
	}
	var key *rsa.PublicKey
	for _, k := range jwks.Keys {
		if k.Kid == hdr.Kid {
			pk, err := k.publicKey()
			if err != nil {
				return c, err
			}
			key = pk
		}
	}
	if key == nil {
		return c, fmt.Errorf("unknown kid %q", hdr.Kid)
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return c, fmt.Errorf("decode signature: %w", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return c, fmt.Errorf("bad signature: %w", err)
	}
	if err := decodeSegment(parts[1], &c); err != nil {
		return c, err
	}
	if now.Unix() >= c.ExpiresAt {
		return c, errors.New("token expired")
	}
	return c, nil
}

func decodeSegment(seg string, v any) error {
	raw, err := b64.DecodeString(seg)
	if err != nil {
		return fmt.Errorf("decode segment: %w", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("unmarshal segment: %w", err)
	}
	return nil
}

func (k JWK) publicKey() (*rsa.PublicKey, error) {
	n, err := b64.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	e, err := b64.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	ei := new(big.Int).SetBytes(e)
	if !ei.IsInt64() || ei.Int64() < 3 || ei.Int64() > math.MaxInt32 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(ei.Int64())}, nil
}

// Set is the created test material: the trusted JWKS and named tokens.
type Set struct {
	JWKS   JWKS
	Tokens map[string]string
}

// Generate mints the full set of tokens for issuer and audience at time now.
//
//	valid-customer  alice, role=customer: may POST /checkout
//	valid-analyst   bob, role=analyst: may also GET /reports
//	expired         alice, expired an hour ago
//	forged          alice, signed by an attacker key that reuses the real kid
//	wrong-issuer    alice, issued by an untrusted issuer
//	wrong-audience  alice, minted for another application
//	alg-none        alice, unsigned ("alg": "none")
//	tampered        valid-customer with the payload edited to role=analyst
func Generate(issuer, audience string, now time.Time) (*Set, error) {
	const kid = "zt-demo-1"
	good, err := NewSigner(kid)
	if err != nil {
		return nil, err
	}
	evil, err := NewSigner(kid)
	if err != nil {
		return nil, err
	}
	base := func(sub, role string) Claims {
		return Claims{
			Issuer: issuer, Subject: sub, Audience: audience, Role: role,
			IssuedAt: now.Unix(), NotBefore: now.Add(-time.Minute).Unix(),
			ExpiresAt: now.Add(2 * time.Hour).Unix(),
		}
	}
	expired := base("alice", "customer")
	expired.IssuedAt = now.Add(-3 * time.Hour).Unix()
	expired.NotBefore = expired.IssuedAt
	expired.ExpiresAt = now.Add(-time.Hour).Unix()
	wrongIss := base("alice", "customer")
	wrongIss.Issuer = "https://evil.example"
	wrongAud := base("alice", "customer")
	wrongAud.Audience = "some-other-app"

	set := &Set{JWKS: good.PublicJWKS(), Tokens: map[string]string{}}
	signed := []struct {
		name string
		s    *Signer
		c    Claims
	}{
		{"valid-customer", good, base("alice", "customer")},
		{"valid-analyst", good, base("bob", "analyst")},
		{"expired", good, expired},
		{"forged", evil, base("alice", "analyst")},
		{"wrong-issuer", good, wrongIss},
		{"wrong-audience", good, wrongAud},
	}
	for _, t := range signed {
		tok, err := t.s.Sign(t.c)
		if err != nil {
			return nil, err
		}
		set.Tokens[t.name] = tok
	}
	none, err := Unsigned(base("alice", "analyst"))
	if err != nil {
		return nil, err
	}
	set.Tokens["alg-none"] = none

	parts := strings.Split(set.Tokens["valid-customer"], ".")
	edited, err := encodeSegment(base("alice", "analyst"))
	if err != nil {
		return nil, err
	}
	set.Tokens["tampered"] = parts[0] + "." + edited + "." + parts[2]
	return set, nil
}

// Write stores jwks.json and one <name>.jwt file per token in dir.
func (s *Set) Write(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	raw, err := json.Marshal(s.JWKS)
	if err != nil {
		return nil, fmt.Errorf("marshal jwks: %w", err)
	}
	files := []string{filepath.Join(dir, "jwks.json")}
	if err := os.WriteFile(files[0], raw, 0o600); err != nil {
		return nil, fmt.Errorf("write jwks: %w", err)
	}
	names := make([]string, 0, len(s.Tokens))
	for n := range s.Tokens {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(dir, n+".jwt")
		if err := os.WriteFile(p, []byte(s.Tokens[n]), 0o600); err != nil {
			return nil, fmt.Errorf("write %s: %w", p, err)
		}
		files = append(files, p)
	}
	return files, nil
}
