package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// operatorAuthorized 校验请求头里的 Bearer access_token 是否有效。
func operatorAuthorized(r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	_, ok := verifyAccessToken(token)
	return ok
}

func initOperatorAuth() {
	// JWT 密钥
	ensureJWTSecret()

	// TOTP 仅在有 OPERATOR_TOTP_SECRET 时才启用
	// 不自动打印 URI，不自动生成 secret
	// 这是 fail-closed 设计：默认不开双因素
}

// handleLogin 双因素或单因素登录
// 请求体: { "username": string, "password": string, "code": string }
// - username 必须与 OPERATOR_USERNAME 匹配
// - password 必须与 OPERATOR_PASSWORD 匹配
// - code 仅在设置了 OPERATOR_TOTP_SECRET 时校验
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// 校验用户名 + 密码
	if body.Username != operatorUser || body.Password != operatorPass {
		http.Error(w, "用户名或密码错误", http.StatusUnauthorized)
		return
	}

	// 校验 TOTP（仅当 OPERATOR_TOTP_SECRET 已设置时）
	totpSecret := os.Getenv("OPERATOR_TOTP_SECRET")
	if totpSecret != "" {
		if !ValidateTOTP(totpSecret, body.Code) {
			http.Error(w, "动态码错误", http.StatusUnauthorized)
			return
		}
	}

	// 记录登录日志（IP/地点/设备/用户名），地点解析走网络故异步
	ip := clientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	ua := r.UserAgent()
	go insertPanelLogin(time.Now().Unix(), ip, lookupLocation(ip), parseDevice(ua), body.Username)

	// 签发 token
	accessToken, refreshToken, err := generateTokenSet()
	if err != nil {
		http.Error(w, "token generation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	newAccess, newRefresh, ok := refreshOperatorToken(body.RefreshToken)
	if !ok {
		http.Error(w, "refresh token 无效或已吊销", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  newAccess,
		"refresh_token": newRefresh,
	})
}

// authenticateOperatorWS 从 /ws/operator 的 query/header 验证 JWT access_token
func authenticateOperatorWS(r *http.Request) bool {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
	}
	_, ok := verifyAccessToken(token)
	return ok
}
