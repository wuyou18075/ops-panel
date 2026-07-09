package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// 命令签名逻辑与 Master 端保持一致（同一算法、同一 secret）。
// Master 用 signCommand 下发签名，Agent 用 verifyCommand 验签。

func signCommand(secret, agentID, data string, nonce int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(agentID))
	mac.Write([]byte(data))
	mac.Write([]byte(time.Unix(nonce, 0).Format(time.RFC3339)))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyCommand 校验签名并防重放（±2 分钟时间窗）。
func verifyCommand(secret, agentID, data string, nonce int64, sig string) bool {
	if diff := time.Since(time.Unix(nonce, 0)); diff < -2*time.Minute || diff > 2*time.Minute {
		return false
	}
	expected := signCommand(secret, agentID, data, nonce)
	return hmac.Equal([]byte(expected), []byte(sig))
}
