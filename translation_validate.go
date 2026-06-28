package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type translationValidationError struct {
	RowNumber int
	Err       error
}

func (err *translationValidationError) Error() string {
	return fmt.Sprintf("第 %d 条翻译校验失败: %v", err.RowNumber, err.Err)
}

func (err *translationValidationError) Unwrap() error {
	return err.Err
}

func hasUsableExistingTranslation(row LocalizationRow) bool {
	if strings.TrimSpace(row.Schinese) == "" {
		return false
	}
	if requiresChineseTranslation(row.English) && !hanCharRegex.MatchString(row.Schinese) {
		return false
	}
	return true
}

var protectedTokenRegex = regexp.MustCompile(`\[[A-Fa-f0-9]{6,8}\]|\[-\]|%[+#\-0-9.]*[A-Za-z]|\{\d+\}`)

var hanCharRegex = regexp.MustCompile(`\p{Han}`)

var englishWordRegex = regexp.MustCompile(`[A-Za-z]+`)

func validateTranslationResult(src string, dst string, unchanged bool) error {
	if strings.TrimSpace(dst) == "" {
		return errors.New("译文为空")
	}
	if err := validateProtectedTokens(src, dst); err != nil {
		return err
	}
	if unchanged {
		if strings.TrimSpace(src) != strings.TrimSpace(dst) {
			return errors.New("AI 标记无需翻译，但译文与原文不一致")
		}
		return nil
	}
	if requiresChineseTranslation(src) && !hanCharRegex.MatchString(dst) {
		return errors.New("译文不包含中文字符，疑似未翻译或被相邻条目污染")
	}
	return nil
}

func validateProtectedTokens(src string, dst string) error {
	srcTokens := protectedTokenRegex.FindAllString(src, -1)
	for _, token := range srcTokens {
		if !strings.Contains(dst, token) {
			return fmt.Errorf("缺少 token %s", token)
		}
	}
	return nil
}

func requiresChineseTranslation(src string) bool {
	clean := protectedTokenRegex.ReplaceAllString(src, " ")
	words := englishWordRegex.FindAllString(clean, -1)
	for _, word := range words {
		if len(word) >= 4 {
			return true
		}
		if word != strings.ToUpper(word) {
			return true
		}
	}
	return false
}
