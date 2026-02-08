package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type Claims struct {
	APIKey        string   `json:"api_key"`
	RateLimit     int      `json:"rate_limit"`
	ExpiresAtRFC  string   `json:"expires_at"`
	AllowedRoutes []string `json:"allowed_routes"`

	jwt.RegisteredClaims
}

func main() {
	redisAddr := flag.String("redis", "localhost:6379", "Redis address")
	prefix := flag.String("prefix", "token:", "Redis key prefix (token:<api_key>)")
	secret := flag.String("secret", "", "JWT HS256 secret (required)")
	limit := flag.Int("limit", 10, "Rate limit for api_key")
	ttl := flag.Duration("ttl", 24*time.Hour, "Token TTL")
	routes := flag.String("routes", "/api/v1/test,/api/v1/test2,", "Comma-separated allowed routes")
	flag.Parse()

	if *secret == "" {
		log.Fatal("flag -secret is required")
	}

	if *limit <= 0 {
		log.Fatal("flag -limit must be > 0")
	}

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	defer rdb.Close()

	apiKey, err := GenerateAPIKey()
	if err != nil {
		log.Fatalf("Failed to generate api_key: %v", err)
	}

	expiresAt := time.Now().UTC().Add(*ttl)
	allowed := splitCSV(*routes)

	// JWT claims: include both expires_at (RFC3339 string) and standard exp (NumericDate)
	claims := Claims{
		APIKey:        apiKey,
		RateLimit:     *limit,
		ExpiresAtRFC:  expiresAt.Format(time.RFC3339),
		AllowedRoutes: allowed,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt), // standard "exp"
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}

	j := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwtStr, err := j.SignedString([]byte(*secret))
	if err != nil {
		log.Fatalf("Failed to sign JWT: %v", err)
	}

	// Store in Redis by api_key
	key := *prefix + apiKey
	allowedJSON, _ := json.Marshal(allowed)

	pipe := rdb.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"api_key":        apiKey,
		"rate_limit":     fmt.Sprintf("%d", *limit),
		"expires_at":     expiresAt.Format(time.RFC3339),
		"allowed_routes": string(allowedJSON),
	})
	pipe.ExpireAt(ctx, key, expiresAt)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Fatalf("Failed to save token profile: %v", err)
	}

	fmt.Println("\nToken created successfully!")
	fmt.Printf("\napi_key: %s\n", apiKey)
	fmt.Printf("\nstorage_key: %s\n", key)
	fmt.Printf("jwt: %s\n", jwtStr)
	fmt.Printf("\nexpires_at: %s\n", expiresAt.Format(time.RFC3339))
	fmt.Printf("\nalowed routes: %s\n", string(allowedJSON))
	fmt.Printf("curl example:\n\n")
	fmt.Printf("curl -H 'Authorization: Bearer %s' http://localhost:8080/api/v1/test\n", jwtStr)
}

// simple short api key. We use JWT token encoding later
func GenerateAPIKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}
