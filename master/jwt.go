package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtPayload struct {
	Sub  string `json:"sub"`
	Exp  int64  `json:"exp"`
	Iat  int64  `json:"iat"`
	Type string `json:"type"`
}

var (
	jwtSecret      = ""
	jwtSecretMu    sync.Once
	refreshTokens  = map[string]bool{}
	refreshTokenMu sync.RWMutex
)

func ensureJWTSecret() {
	jwtSecretMu.Do(func() {
		if s := os.Getenv("JWT_SECRET"); s != "" {
			jwtSecret = s
		} else {
			jwtSecret = genSecret()
			fmt.Printf("[JWT] 未设置 JWT_SECRET，已自动生成: %s\n请写入环境变量以保持重启后 token 有效。\n", jwtSecret)
		}
	})
}

func generateTokenSet() (accessToken, refreshToken string, err error) {
	ensureJWTSecret()
	now := time.Now()
	accessToken, err = signToken(jwtPayload{
		Sub: "operator", Exp: now.Add(15 * time.Minute).Unix(),
		Iat: now.Unix(), Type: "access",
	})
	if err != nil {
		return "", "", err
	}
	refreshToken, err = signToken(jwtPayload{
		Sub: "operator", Exp: now.Add(7 * 24 * time.Hour).Unix(),
		Iat: now.Unix(), Type: "refresh",
	})
	if err != nil {
		return "", "", err
	}
	refreshTokenMu.Lock()
	refreshTokens[refreshToken] = true
	refreshTokenMu.Unlock()
	return accessToken, refreshToken, nil
}

func signToken(p jwtPayload) (string, error) {
	headerRaw, _ := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	payloadRaw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	headerB64 := base64.RawURLEncoding.EncodeToString(headerRaw)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadRaw)
	sigInput := headerB64 + "." + payloadB64

	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(sigInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return sigInput + "." + sig, nil
}

func verifyJWT(token string) (*jwtPayload, bool) {
	ensureJWTSecret()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	sigInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write([]byte(sigInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return nil, false
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var p jwtPayload
	if err := json.Unmarshal(payloadRaw, &p); err != nil {
		return nil, false
	}
	if time.Now().Unix() > p.Exp {
		return nil, false
	}
	return &p, true
}

func verifyAccessToken(token string) (*jwtPayload, bool) {
	p, ok := verifyJWT(token)
	if !ok || p.Type != "access" {
		return nil, false
	}
	return p, true
}

func revokeRefreshToken(token string) {
	refreshTokenMu.Lock()
	delete(refreshTokens, token)
	refreshTokenMu.Unlock()
}

func refreshOperatorToken(refreshToken string) (newAccess, newRefresh string, ok bool) {
	p, valid := verifyJWT(refreshToken)
	if !valid || p.Type != "refresh" {
		return "", "", false
	}
	refreshTokenMu.RLock()
	stillValid := refreshTokens[refreshToken]
	refreshTokenMu.RUnlock()
	if !stillValid {
		return "", "", false
	}
	revokeRefreshToken(refreshToken)
	a, r, err := generateTokenSet()
	if err != nil {
		return "", "", false
	}
	return a, r, true
}
