package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// Sign 为私有文件签发访问签名。
//
// 签名同时覆盖文件 key 与过期时间，因此把 URL 里的过期时间往后改、
// 或者把 key 换成别人的文件，都会导致签名校验失败。
func Sign(secret, key string, expireAt int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(key))
	mac.Write([]byte{'\n'})
	mac.Write([]byte(strconv.FormatInt(expireAt, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify 校验签名与有效期。
func Verify(secret, key, expireAtRaw, sig string) error {
	expireAt, err := strconv.ParseInt(expireAtRaw, 10, 64)
	if err != nil {
		return errcode.ErrFileSignInvalid
	}
	if time.Now().Unix() > expireAt {
		return errcode.ErrFileSignExpired
	}
	// 用常量时间比较，避免通过响应耗时逐字节猜出签名。
	if !hmac.Equal([]byte(Sign(secret, key, expireAt)), []byte(sig)) {
		return errcode.ErrFileSignInvalid
	}
	return nil
}
