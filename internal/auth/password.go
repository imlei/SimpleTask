package auth

import (
	"errors"
	"fmt"
	"regexp"
	"unicode"
)

var (
	ErrPasswordTooShort    = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong     = errors.New("password must not exceed 128 characters")
	ErrPasswordNoDigit     = errors.New("password must contain at least one digit")
	ErrPasswordNoUpper     = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLower     = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoSpecial   = errors.New("password must contain at least one special character")
	ErrPasswordContainsSeq = errors.New("password must not contain common sequences")
	ErrPasswordTooWeak     = errors.New("password is too weak")
)

// 密码复杂度要求
const (
	minPasswordLength = 8
	maxPasswordLength = 128
	bcryptCost        = 12 // 增加 bcrypt 成本以提高安全性
)

// 常见的弱密码序列
var commonSequences = []string{
	"123", "234", "345", "456", "567", "678", "789", "890", "012",
	"abc", "bcd", "cde", "def", "efg", "fgh", "ghi", "hij", "ijk",
	"qwe", "wer", "ert", "rty", "tyu", "yui", "uio", "iop",
	"asd", "sdf", "dfg", "fgh", "ghj", "hjk", "jkl",
	"zxc", "xcv", "cvb", "vbn", "bnm",
	"password", "admin", "123456", "qwerty", "letmein",
}

// ValidatePasswordStrength 验证密码强度
func ValidatePasswordStrength(password string) error {
	if len(password) < minPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > maxPasswordLength {
		return ErrPasswordTooLong
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasDigit   bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return ErrPasswordNoUpper
	}
	if !hasLower {
		return ErrPasswordNoLower
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}
	if !hasSpecial {
		return ErrPasswordNoSpecial
	}

	// 检查常见序列
	passwordLower := toLower(password)
	for _, seq := range commonSequences {
		if len(seq) >= 3 && len(password) >= len(seq) {
			for i := 0; i <= len(passwordLower)-len(seq); i++ {
				if passwordLower[i:i+len(seq)] == seq {
					return ErrPasswordContainsSeq
				}
			}
		}
	}

	// 检查重复字符（超过3个相同字符连续）
	if matches, _ := regexp.MatchString(`(.)\1{3,}`, password); matches {
		return ErrPasswordTooWeak
	}

	// 检查键盘模式
	if containsKeyboardPattern(passwordLower) {
		return ErrPasswordTooWeak
	}

	return nil
}

// toLower 转换为小写
func toLower(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		result[i] = unicode.ToLower(r)
	}
	return string(result)
}

// containsKeyboardPattern 检查键盘模式
func containsKeyboardPattern(s string) bool {
	// 简单的键盘行检测
	rows := []string{
		"qwertyuiop",
		"asdfghjkl",
		"zxcvbnm",
	}

	for _, row := range rows {
		for i := 0; i <= len(row)-4; i++ {
			sequence := row[i : i+4]
			// 正序
			for j := 0; j <= len(s)-4; j++ {
				if s[j:j+4] == sequence {
					return true
				}
			}
			// 反序
			rev := reverse(sequence)
			for j := 0; j <= len(s)-4; j++ {
				if s[j:j+4] == rev {
					return true
				}
			}
		}
	}
	return false
}

// reverse 反转字符串
func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// GetPasswordStrengthHint 获取密码强度提示
func GetPasswordStrengthHint() string {
	return fmt.Sprintf("Password must be %d-%d characters and contain at least one uppercase letter, one lowercase letter, one digit, and one special character",
		minPasswordLength, maxPasswordLength)
}
