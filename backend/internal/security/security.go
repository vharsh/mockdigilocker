// Package security contains crypto helpers: random identifiers, password
// hashing (PBKDF2-SHA256), PKCE verification, HMAC signatures and JWTs.
package security

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// RandomDigits returns n random decimal digits (for OTPs).
func RandomDigits(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		v, _ := rand.Int(rand.Reader, big.NewInt(10))
		sb.WriteString(v.String())
	}
	return sb.String()
}

// UUID returns a random RFC 4122 v4 UUID (36 chars, like a digilockerid).
func UUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

const pbkdf2Iter = 120000

// HashPassword returns "pbkdf2-sha256$iter$salt$hash".
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(dk)), nil
}

func CheckPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(dk, want) == 1
}

// PKCEChallenge computes BASE64URL-NO-PADDING(SHA256(verifier)).
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

var ErrPKCE = errors.New("code_verifier does not match code_challenge")

// VerifyPKCE checks a verifier against a challenge (S256 or plain).
func VerifyPKCE(challenge, method, verifier string) error {
	if challenge == "" {
		return nil
	}
	if l := len(verifier); l < 43 || l > 128 {
		return errors.New("code_verifier must be 43-128 characters")
	}
	var got string
	switch strings.ToUpper(method) {
	case "", "S256":
		got = PKCEChallenge(verifier)
	case "PLAIN":
		got = verifier
	default:
		return errors.New("unsupported code_challenge_method")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) != 1 {
		return ErrPKCE
	}
	return nil
}

// HMACBase64 is Base64(HMAC-SHA256(key, data)) — DigiLocker's `hmac` header.
func HMACBase64(key string, data []byte) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write(data)
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func HMACHex(key string, data []byte) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}

// SHA256Hex returns hex(SHA256(s)).
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func ConstantEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
