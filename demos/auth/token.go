package main

// Session tokens via the accounts module (EdDSA/Ed25519, byte-identical Go/Python, opaque ids only).
// Thin adapter so the service keeps one Signer type.

import (
	"encoding/base64"
	"errors"
	"time"

	accounts "github.com/sedyukov/sedlab-modules/modules/accounts/go"
	akit "github.com/sedyukov/sedlab-modules/modules/accounts/go/kit"
)

var b64 = base64.RawURLEncoding

type Claims struct {
	Sub  string
	Sid  string
	Prov string
}

var (
	errTokenMalformed = errors.New("token malformed")
	errTokenSignature = errors.New("token signature invalid")
	errTokenExpired   = errors.New("token expired")
)

type Signer struct {
	s   *accounts.Signer
	iss string
}

func NewSigner(key, issuer string) (*Signer, error) {
	s, err := accounts.NewSigner(key)
	if err != nil {
		return nil, err
	}
	s.Issuer = issuer
	return &Signer{s: s, iss: issuer}, nil
}

// GenerateKeypair → (private seed, public key, kid), base64url.
func GenerateKeypair() (seed, pub, kid string) {
	k, err := accounts.GenerateKeypair()
	if err != nil {
		panic(err)
	}
	return k.PrivateKey, k.PublicKey, k.Kid
}

func (s *Signer) PublicKey() string   { return s.s.PublicKey() }
func (s *Signer) Kid() string         { return s.s.Kid() }
func (s *Signer) JWKS() accounts.JWKS { return s.s.JWKS() }

func (s *Signer) Issue(userID, sid, prov string, iat, exp int64) string {
	t, err := s.s.IssueToken(accounts.IssueTokenInput{UserID: userID, SID: sid, Prov: prov, TTLSec: exp - iat, Now: iat})
	if err != nil {
		return ""
	}
	return t.Token
}

func (s *Signer) Verify(token string, now int64) (*Claims, error) {
	id, err := accounts.VerifyTokenWithKey(token, s.s.PublicKey(), now, s.iss)
	if err != nil {
		var ke *akit.Error
		if errors.As(err, &ke) {
			switch ke.Code {
			case akit.Unauthenticated:
				return nil, errTokenExpired
			case akit.SignatureInvalid:
				return nil, errTokenSignature
			}
		}
		return nil, errTokenMalformed
	}
	return &Claims{Sub: id.UserID, Sid: id.SID, Prov: id.Prov}, nil
}

const clockSkew = int64(time.Minute / time.Second)
