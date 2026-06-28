package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func buildTranslationBatches(rows []LocalizationRow, batchSize int) [][]LocalizationRow {
	if batchSize <= 0 {
		batchSize = 1
	}

	var batches [][]LocalizationRow
	normal := make([]LocalizationRow, 0, batchSize)
	flushNormal := func() {
		if len(normal) == 0 {
			return
		}
		batch := append([]LocalizationRow(nil), normal...)
		batches = append(batches, batch)
		normal = normal[:0]
	}

	for _, row := range rows {
		if shouldUseSingleItemBatch(row) {
			flushNormal()
			batches = append(batches, []LocalizationRow{row})
			continue
		}
		normal = append(normal, row)
		if len(normal) >= batchSize {
			flushNormal()
		}
	}
	flushNormal()
	return batches
}

func shouldUseSingleItemBatch(row LocalizationRow) bool {
	key := strings.ToLower(row.Key)
	if strings.HasPrefix(key, "quest") || strings.Contains(key, "_obj") || strings.Contains(key, "objective") {
		return true
	}

	text := strings.TrimSpace(row.English)
	if text == "" {
		return false
	}
	if strings.Contains(text, "\n") || strings.Contains(text, "\r") {
		return true
	}

	plainText := strings.TrimSpace(protectedTokenRegex.ReplaceAllString(text, " "))
	if len([]rune(plainText)) <= 90 && imperativePrefixRegex.MatchString(plainText) {
		return true
	}
	return false
}

func apiEndpoint(baseURL string, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path = strings.TrimLeft(path, "/")
	if strings.HasSuffix(strings.ToLower(baseURL), "/v1") {
		return baseURL + "/" + path
	}
	return baseURL + "/v1/" + path
}

func translateWithRetry(ctx context.Context, opts TranslateOptions, rows []LocalizationRow) ([]string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		translations, err := translateBatch(ctx, opts, rows)
		if err == nil {
			return translations, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func translateRows(ctx context.Context, opts TranslateOptions, rows []LocalizationRow) []translationResult {
	return translateWithFallback(ctx, opts, rows)
}

func translateWithFallback(ctx context.Context, opts TranslateOptions, rows []LocalizationRow) []translationResult {
	translations, err := translateWithRetry(ctx, opts, rows)
	if err == nil {
		return []translationResult{{rows: rows, translations: translations}}
	}
	if ctx.Err() != nil || len(rows) <= 1 {
		return []translationResult{{rows: rows, err: err}}
	}

	var validationErr *translationValidationError
	if !errors.As(err, &validationErr) {
		return []translationResult{{rows: rows, err: err}}
	}

	mid := len(rows) / 2
	results := translateWithFallback(ctx, opts, rows[:mid])
	results = append(results, translateWithFallback(ctx, opts, rows[mid:])...)
	return results
}

func translateBatch(ctx context.Context, opts TranslateOptions, rows []LocalizationRow) ([]string, error) {
	inputs := make([]translationRequestItem, len(rows))
	for i, row := range rows {
		inputs[i] = translationRequestItem{
			ID:   row.Index,
			Key:  row.Key,
			Text: row.English,
		}
	}

	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"model":       opts.Model,
		"temperature": 0.2,
		"stream":      false,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": translationSystemPrompt(),
			},
			{
				"role":    "user",
				"content": "请翻译以下 JSON 对象数组。每个对象的 key 只用于定位和少量上下文，不要翻译 key；只处理 text。只返回同顺序 JSON 对象数组，每个对象格式必须是 {\"id\": 原 id, \"translation\": \"简体中文译文或原文\", \"unchanged\": false}。如果 text 是网址、账号、版本号、文件名、代码片段、纯符号、纯占位符等不需要本地化的内容，请把 translation 原样设为 text，并把 unchanged 设为 true；其他需要翻译的英文文本必须把 unchanged 设为 false，并翻译成简体中文：\n" + string(inputJSON),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint(opts.BaseURL, "chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opts.APIKey))

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("翻译请求失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("无法解析 Chat Completions 响应: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, errors.New("AI 响应中没有 choices")
	}

	items, err := parseTranslationObjectItems(chatResp.Choices[0].Message.Content, rows)
	if err != nil {
		return nil, err
	}
	translations := translationsFromItems(items)
	if len(translations) != len(rows) {
		return nil, fmt.Errorf("AI 返回数量不一致: 输入 %d 条，返回 %d 条", len(rows), len(translations))
	}
	for i := range translations {
		if err := validateTranslationResult(rows[i].English, translations[i], items[i].Unchanged); err != nil {
			return nil, &translationValidationError{RowNumber: rows[i].RowNumber, Err: err}
		}
	}
	return translations, nil
}

var imperativePrefixRegex = regexp.MustCompile(`(?i)^\s*(activate|bring|clear|collect|craft|defend|destroy|drop|finish|get|gather|hold|hunt|kill|locate|open|place|reach|repair|retrieve|return|set|survive|talk|trigger|travel|use)\b`)
