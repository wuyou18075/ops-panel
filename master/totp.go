package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// TOTP 双因素认证：绑定 Google Authenticator。
// 运维登录指挥台需：密码(operator_token) + 6位动态码(TOTP) 同时通过。
// 即使密码泄露，无动态码也无法下发命令。

const totpDigits = 6
const totpPeriod = 30 // 秒

// GenerateTOTPSecret 生成新的 Base32 密钥与 otpauth:// 绑定 URI。
// account 用于 App 中显示的账号名。
func GenerateTOTPSecret(account string) (secret string, uri string) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	uri = fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=OpsPanel&period=%d&digits=%d",
		account, secret, totpPeriod, totpDigits)
	return secret, uri
}

// ValidateTOTP 校验 6 位动态码，容忍前后一个时间窗（网络延迟）。
func ValidateTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	counter := time.Now().Unix() / totpPeriod
	for _, c := range []int64{counter - 1, counter, counter + 1} {
		if hotp(secret, c) == code {
			return true
		}
	}
	return false
}

// hotp 计算指定时间窗的 HOTP 值（RFC 4226）。
func hotp(secret string, counter int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		// 兼容带填充的标准格式
		key, err = base32.StdEncoding.DecodeString(secret)
		if err != nil {
			return ""
		}
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, code%mod)
}
