package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	APIKey        string   `json:"api_key"`
	AllowedRoutes []string `json:"allowed_routes,omitempty"`
	RateLimit     int      `json:"rate_limit,omitempty"`

	ExpiresAtRFC3339 string `json:"expires_at,omitempty"`

	jwt.RegisteredClaims
}

type KeySet struct {
	ExpectedAlg string // e.g. "HS256" or "RS256"
	DefaultKey  any    // HS256: []byte(secret), RS256: *rsa.PublicKey
}

type JWTVerifier struct {
	ks KeySet
}

func NewJWTVerifier(ks KeySet) *JWTVerifier {
	return &JWTVerifier{
		ks: ks,
	}
}

func (v *JWTVerifier) Parse(tokenString string) (*Claims, error) {
	if v.ks.DefaultKey == nil {
		return nil, errors.New("no key configured")
	}

	if v.ks.ExpectedAlg == "" {
		return nil, errors.New("expected algorithm not configured")
	}

	claims := new(Claims)

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{v.ks.ExpectedAlg}),
		jwt.WithExpirationRequired(),
	)

	tok, err := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != v.ks.ExpectedAlg {
			return nil, fmt.Errorf("unexpected alg: %s", t.Method.Alg())
		}
		return v.ks.DefaultKey, nil
	})
	if err != nil {
		return nil, err
	}

	if tok == nil || !tok.Valid {
		return nil, errors.New("invalid token")
	}

	if claims.APIKey == "" {
		return nil, errors.New("missing api_key claim")
	}

	return claims, nil
}
