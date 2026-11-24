package security

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("123456")
	if err != nil || !strings.HasPrefix(h, "pbkdf2-sha256$") {
		t.Fatal(err, h)
	}
	if !CheckPassword(h, "123456") || CheckPassword(h, "123457") || CheckPassword("garbage", "x") {
		t.Fatal("check failed")
	}
}

func TestPKCE(t *testing.T) {
	// expected value computed independently: base64url(sha256(v)) without padding
	v := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r7wW1gFWFOEjXk"
	if c := PKCEChallenge(v); c != "bwWFMyPfdG9qreDhH2lmftFx_dFeLDalzcT1gb_j68g" {
		t.Fatalf("challenge %s", c)
	}
	if VerifyPKCE("bwWFMyPfdG9qreDhH2lmftFx_dFeLDalzcT1gb_j68g", "S256", v) != nil {
		t.Fatal("verify")
	}
	if VerifyPKCE("bwWFMyPfdG9qreDhH2lmftFx_dFeLDalzcT1gb_j68g", "S256", strings.Repeat("a", 43)) == nil {
		t.Fatal("wrong verifier accepted")
	}
	if VerifyPKCE("x", "S256", "short") == nil {
		t.Fatal("short verifier accepted")
	}
}

func TestJWT(t *testing.T) {
	tok, _ := SignHS256("k", map[string]any{"sub": "a", "exp": time.Now().Add(time.Minute).Unix()})
	if c, err := VerifyHS256("k", tok); err != nil || c["sub"] != "a" {
		t.Fatal(err)
	}
	if _, err := VerifyHS256("other", tok); err == nil {
		t.Fatal("wrong key accepted")
	}
	exp, _ := SignHS256("k", map[string]any{"exp": time.Now().Add(-time.Minute).Unix()})
	if _, err := VerifyHS256("k", exp); err == nil {
		t.Fatal("expired accepted")
	}
	k, _ := LoadOrGenerateRSA("")
	rs, _ := k.SignRS256(map[string]any{"iss": "x"})
	if c, err := k.VerifyRS256(rs); err != nil || c["iss"] != "x" {
		t.Fatal(err)
	}
	if len(k.JWKS()["keys"].([]map[string]any)) != 1 {
		t.Fatal("jwks")
	}
}

func TestUUID(t *testing.T) {
	u := UUID()
	if len(u) != 36 || u[14] != '4' {
		t.Fatal(u)
	}
}
