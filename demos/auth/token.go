package main

// Session tokens: compact JWS, EdDSA (Ed25519). Format is fixed by accounts/CONTRACT (module `accounts`):
// header {"alg":"EdDSA","typ":"JWT","kid":…}, payload keys in the order iss,sub,sid,prov,iat,exp,v, no spaces.
// No personal data in the token: it travels to demo servers outside RU.
// TODO(accounts module): replace with accounts.IssueToken/VerifyToken once s2 ships the module.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

var b64 = base64.RawURLEncoding

type Claims struct {
	Iss  string `json:"iss"`
	Sub  string `json:"sub"`
	Sid  string `json:"sid"`
	Prov string `json:"prov"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
	V    int    `json:"v"`
}

var (
	errTokenMalformed = errors.New("token malformed")
	errTokenSignature = errors.New("token signature invalid")
	errTokenExpired   = errors.New("token expired")
)

const clockSkew = 60

type Signer struct {
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
	kid  string
	iss  string
}

// kidOf: first 8 bytes of SHA-256 of the public key, base64url.
func kidOf(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return b64.EncodeToString(h[:8])
}

// NewSigner takes a base64url Ed25519 seed (32 bytes) or full private key (64 bytes).
func NewSigner(key, issuer string) (*Signer, error) {
	raw, err := b64.DecodeString(strings.TrimRight(strings.TrimSpace(key), "="))
	if err != nil {
		return nil, errors.New("AUTH_SIGNING_KEY: not base64url")
	}
	var priv ed25519.PrivateKey
	switch len(raw) {
	case ed25519.SeedSize:
		priv = ed25519.NewKeyFromSeed(raw)
	case ed25519.PrivateKeySize:
		priv = ed25519.PrivateKey(raw)
	default:
		return nil, errors.New("AUTH_SIGNING_KEY: want 32-byte seed or 64-byte key")
	}
	pub := priv.Public().(ed25519.PublicKey)
	return &Signer{priv: priv, pub: pub, kid: kidOf(pub), iss: issuer}, nil
}

func GenerateKeypair() (seed, pub, kid string) {
	p, s, _ := ed25519.GenerateKey(rand.Reader)
	return b64.EncodeToString(s.Seed()), b64.EncodeToString(p), kidOf(p)
}

func (s *Signer) PublicKey() string { return b64.EncodeToString(s.pub) }
func (s *Signer) Kid() string       { return s.kid }

func (s *Signer) Issue(userID, sid, prov string, iat, exp int64) string {
	hdr := `{"alg":"EdDSA","typ":"JWT","kid":` + strconv.Quote(s.kid) + `}`
	pl := `{"iss":` + strconv.Quote(s.iss) + `,"sub":` + strconv.Quote(userID) + `,"sid":` + strconv.Quote(sid) +
		`,"prov":` + strconv.Quote(prov) + `,"iat":` + strconv.FormatInt(iat, 10) + `,"exp":` + strconv.FormatInt(exp, 10) + `,"v":1}`
	in := b64.EncodeToString([]byte(hdr)) + "." + b64.EncodeToString([]byte(pl))
	return in + "." + b64.EncodeToString(ed25519.Sign(s.priv, []byte(in)))
}

func (s *Signer) Verify(token string, now int64) (*Claims, error) {
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errTokenMalformed
	}
	hb, err1 := b64.DecodeString(parts[0])
	pb, err2 := b64.DecodeString(parts[1])
	sig, err3 := b64.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, errTokenMalformed
	}
	var h struct{ Alg, Kid string }
	if json.Unmarshal(hb, &h) != nil {
		return nil, errTokenMalformed
	}
	if h.Alg != "EdDSA" || h.Kid != s.kid || !ed25519.Verify(s.pub, []byte(parts[0]+"."+parts[1]), sig) {
		return nil, errTokenSignature
	}
	var c Claims
	if json.Unmarshal(pb, &c) != nil || c.Sub == "" || c.Sid == "" {
		return nil, errTokenMalformed
	}
	if c.Iss != s.iss {
		return nil, errTokenSignature
	}
	if now > c.Exp+clockSkew {
		return nil, errTokenExpired
	}
	return &c, nil
}
