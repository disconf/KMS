// Package hash 提供口令哈希能力，用于管理员账号等需要密码的场景。
package hash

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// Hash 生成 bcrypt 哈希。
func Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", errcode.ErrInternal.Wrap(fmt.Errorf("生成口令哈希失败: %w", err))
	}
	return string(b), nil
}

// Verify 校验明文与哈希是否匹配。
func Verify(hashed, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}
