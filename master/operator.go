package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

var (
	operatorPassword   string
	operatorTOTPSecret string
)

func initOperatorAuth() {
	operatorPassword = os.Getenv("OPERATOR_TOKEN")
	if operatorPassword == "" {
		operatorPassword = ""
	}

	operatorTOTPSecret = os.Getenv("OPERATOR_TOTP_SECRET")
	if operatorTOTPSecret == "" {
		secret, uri := GenerateTOTPSecret("ops-panel-operator")
		operatorTOTPSecret = secret
		fmt.Printf("[Operator] 未设置 OPERATOR_TOTP_SECRET，已自动生成。\n请在 Google Authenticator 中绑定以下 URI，并将该 secret 写入环境变量后重启：\n%s\n", uri)
	} else {
		fmt.Printf("[Operator] TOTP 双因素认证已启用（secret 已读取）。\n")
	}

	ensureJWTSecret()
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if !authenticateOperator(body.Password, body.Code) {
		http.Error(w, "密码或动态码错误", http.StatusUnauthorized)
		return
	}
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

func authenticateOperator(password, code string) bool {
	if operatorPassword == "" {
		return false
	}
	if password != operatorPassword {
		return false
	}
	return ValidateTOTP(operatorTOTPSecret, code)
}

func authenticateOperatorWS(r *http.Request) bool {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
	}
	_, ok := verifyAccessToken(token)
	return ok
}
