package security

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"strings"
	"time"
)

var ErrInvalidJWT = errors.New("invalid token")

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// SignHS256 creates a compact HS256 JWT.
func SignHS256(secret string, claims map[string]any) (string, error) {
	h, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	p, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := b64(h) + "." + b64(p)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(unsigned))
	return unsigned + "." + b64(m.Sum(nil)), nil
}

// VerifyHS256 validates signature and exp, returning the claims.
func VerifyHS256(secret, tok string) (map[string]any, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidJWT
	}
	var hdr map[string]any
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &hdr) != nil || hdr["alg"] != "HS256" {
		return nil, ErrInvalidJWT
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, m.Sum(nil)) {
		return nil, ErrInvalidJWT
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidJWT
	}
	var claims map[string]any
	if err := json.Unmarshal(pb, &claims); err != nil {
		return nil, ErrInvalidJWT
	}
	if exp, ok := claims["exp"].(float64); ok && time.Now().Unix() > int64(exp) {
		return nil, errors.New("token expired")
	}
	return claims, nil
}

// RSAKey signs id_tokens (RS256) and publishes the JWKS.
type RSAKey struct {
	Key *rsa.PrivateKey
	KID string
}

// LoadOrGenerateRSA loads a PEM private key (PKCS#1 or PKCS#8), or generates
// an ephemeral 2048-bit key when path is empty.
func LoadOrGenerateRSA(path string) (*RSAKey, error) {
	var k *rsa.PrivateKey
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil, errors.New("RSA key file: no PEM block")
		}
		if pk, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
			k = pk
		} else if any8, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
			var ok bool
			if k, ok = any8.(*rsa.PrivateKey); !ok {
				return nil, errors.New("RSA key file: not an RSA key")
			}
		} else {
			return nil, errors.New("RSA key file: unsupported key format")
		}
	} else {
		var err error
		if k, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			return nil, err
		}
	}
	der := x509.MarshalPKCS1PublicKey(&k.PublicKey)
	sum := sha256.Sum256(der)
	return &RSAKey{Key: k, KID: b64(sum[:16])}, nil
}

func (r *RSAKey) SignRS256(claims map[string]any) (string, error) {
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": r.KID})
	p, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := b64(h) + "." + b64(p)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, r.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + b64(sig), nil
}

// VerifyRS256 is used by tests and the built-in demo partner.
func (r *RSAKey) VerifyRS256(tok string) (map[string]any, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidJWT
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidJWT
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(&r.Key.PublicKey, crypto.SHA256, sum[:], sig) != nil {
		return nil, ErrInvalidJWT
	}
	pb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if json.Unmarshal(pb, &claims) != nil {
		return nil, ErrInvalidJWT
	}
	return claims, nil
}

func (r *RSAKey) JWKS() map[string]any {
	pub := r.Key.PublicKey
	return map[string]any{"keys": []map[string]any{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": r.KID,
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}
