package tokens

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	iss = "https://auth.zt-shop.example"
	aud = "zt-shop"
)

func TestGenerateTokenSet(t *testing.T) {
	now := time.Now()
	set, err := Generate(iss, aud, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.JWKS.Keys) != 1 || set.JWKS.Keys[0].Alg != "RS256" || set.JWKS.Keys[0].E != "AQAB" {
		t.Fatalf("jwks %+v", set.JWKS)
	}

	for _, name := range []string{"valid-customer", "valid-analyst"} {
		c, err := Verify(set.Tokens[name], set.JWKS, now)
		if err != nil {
			t.Fatalf("%s must verify: %v", name, err)
		}
		if c.Issuer != iss || c.Audience != aud {
			t.Errorf("%s claims %+v", name, c)
		}
	}
	if c, _ := Verify(set.Tokens["valid-analyst"], set.JWKS, now); c.Role != "analyst" || c.Subject != "bob" {
		t.Errorf("analyst claims %+v", c)
	}

	rejected := map[string]string{
		"expired":  "expired",
		"forged":   "bad signature",
		"alg-none": "unsupported alg",
		"tampered": "bad signature",
	}
	for name, want := range rejected {
		if _, err := Verify(set.Tokens[name], set.JWKS, now); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v, want %q", name, err, want)
		}
	}
	// wrong-issuer and wrong-audience are validly signed; it is the mesh's
	// issuer/audience checks that must reject them.
	for name, check := range map[string]func(Claims) bool{
		"wrong-issuer":   func(c Claims) bool { return c.Issuer != iss },
		"wrong-audience": func(c Claims) bool { return c.Audience != aud },
	} {
		c, err := Verify(set.Tokens[name], set.JWKS, now)
		if err != nil || !check(c) {
			t.Errorf("%s: %+v %v", name, c, err)
		}
	}
}

func TestVerifyErrors(t *testing.T) {
	s, err := NewSigner("k1")
	if err != nil {
		t.Fatal(err)
	}
	jwks := s.PublicJWKS()
	tok, err := s.Sign(Claims{ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	cases := map[string]string{
		"a.b":                              "malformed",
		"!!!." + parts[1] + "." + parts[2]: "decode segment",
		parts[0] + "." + parts[1] + ".!!!": "decode signature",
	}
	for in, want := range cases {
		if _, err := Verify(in, jwks, time.Now()); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", in, err, want)
		}
	}
	other := JWKS{Keys: []JWK{{Kid: "other"}}}
	if _, err := Verify(tok, other, time.Now()); err == nil || !strings.Contains(err.Error(), "unknown kid") {
		t.Errorf("unknown kid: %v", err)
	}
	badE := JWKS{Keys: []JWK{{Kid: "k1", N: jwks.Keys[0].N, E: "AQ"}}}
	if _, err := Verify(tok, badE, time.Now()); err == nil || !strings.Contains(err.Error(), "exponent") {
		t.Errorf("bad exponent: %v", err)
	}
	badN := JWKS{Keys: []JWK{{Kid: "k1", N: "!!", E: "AQAB"}}}
	if _, err := Verify(tok, badN, time.Now()); err == nil {
		t.Error("bad modulus must fail")
	}
}

func TestWrite(t *testing.T) {
	set, err := Generate(iss, aud, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "jwt")
	files, err := set.Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(set.Tokens)+1 {
		t.Fatalf("files %v", files)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "jwks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var jwks JWKS
	if err := json.Unmarshal(raw, &jwks); err != nil || len(jwks.Keys) != 1 {
		t.Fatalf("jwks.json: %v %s", err, raw)
	}
	tok, err := os.ReadFile(filepath.Join(dir, "valid-customer.jwt"))
	if err != nil || string(tok) != set.Tokens["valid-customer"] {
		t.Fatalf("valid-customer.jwt: %v", err)
	}
	// A file where the directory should be makes Write fail.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Write(filepath.Join(blocker, "sub")); err == nil {
		t.Error("expected error writing under a file")
	}
}
