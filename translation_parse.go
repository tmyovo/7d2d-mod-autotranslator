package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (item translationResponseItem) translatedText() string {
	for _, value := range []string{item.Translation, item.Text, item.Schinese, item.Chinese, item.Translated} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseTranslationObjectItems(content string, rows []LocalizationRow) ([]translationResponseItem, error) {
	content = normalizeAIJSONContent(content)

	var items []translationResponseItem
	if err := json.Unmarshal([]byte(content), &items); err != nil {
		segment := extractJSONArray(content)
		if segment == "" {
			return nil, errors.New("AI 未返回可解析的 JSON 对象数组")
		}
		if err := json.Unmarshal([]byte(segment), &items); err != nil {
			return nil, fmt.Errorf("AI 返回 JSON 对象数组解析失败: %w", err)
		}
	}

	if len(items) != len(rows) {
		return nil, fmt.Errorf("AI 返回数量不一致: 输入 %d 条，返回 %d 条", len(rows), len(items))
	}

	for i, item := range items {
		if item.ID != rows[i].Index {
			return nil, fmt.Errorf("AI 返回 id 不一致: 第 %d 个期望 id %d，实际 id %d", i+1, rows[i].Index, item.ID)
		}
		translation := item.translatedText()
		if strings.TrimSpace(translation) == "" {
			return nil, fmt.Errorf("AI 返回第 %d 个对象缺少 translation", i+1)
		}
	}
	return items, nil
}

func translationsFromItems(items []translationResponseItem) []string {
	translations := make([]string, len(items))
	for i, item := range items {
		translations[i] = item.translatedText()
	}
	return translations
}

func normalizeAIJSONContent(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```JSON")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func extractJSONArray(content string) string {
	start := strings.Index(content, "[")
	if start < 0 {
		return ""
	}
	inString := false
	escaped := false
	depth := 0
	for i := start; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	return ""
}
