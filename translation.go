package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) TranslateRow(rowIndex int, opts TranslateOptions) (string, error) {
	if strings.TrimSpace(opts.BaseURL) == "" || strings.TrimSpace(opts.APIKey) == "" || strings.TrimSpace(opts.Model) == "" {
		return "", errors.New("Base URL、API Key、模型都必须填写")
	}

	a.mu.Lock()
	if a.doc == nil {
		a.mu.Unlock()
		return "", errors.New("请先加载 Localization 文件")
	}
	row, err := a.rowByIndexLocked(rowIndex)
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(row.English) == "" {
		return "", errors.New("当前行没有英文原文，无法翻译")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	translations, err := translateWithRetry(ctx, opts, []LocalizationRow{row})
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("单条翻译超时：10 秒内没有返回结果")
		}
		return "", err
	}
	if len(translations) != 1 {
		return "", fmt.Errorf("AI 返回数量不一致: 输入 1 条，返回 %d 条", len(translations))
	}
	return translations[0], nil
}

func (a *App) FetchModels(baseURL string, apiKey string) ([]ModelInfo, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, errors.New("Base URL 不能为空")
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, apiEndpoint(baseURL, "models"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取模型失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var openAI struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &openAI); err == nil && len(openAI.Data) > 0 {
		sort.Slice(openAI.Data, func(i, j int) bool { return openAI.Data[i].ID < openAI.Data[j].ID })
		return openAI.Data, nil
	}

	var direct []ModelInfo
	if err := json.Unmarshal(body, &direct); err == nil {
		sort.Slice(direct, func(i, j int) bool { return direct[i].ID < direct[j].ID })
		return direct, nil
	}

	return nil, errors.New("无法识别模型列表响应格式")
}

func (a *App) StartTranslation(opts TranslateOptions) (TranslationSummary, error) {
	if strings.TrimSpace(opts.BaseURL) == "" || strings.TrimSpace(opts.APIKey) == "" || strings.TrimSpace(opts.Model) == "" {
		return TranslationSummary{}, errors.New("Base URL、API Key、模型都必须填写")
	}
	opts.BatchSize = clamp(opts.BatchSize, 1, 50)
	if opts.BatchSize == 0 {
		opts.BatchSize = 15
	}
	opts.Concurrency = clamp(opts.Concurrency, 1, 20)
	if opts.Concurrency == 0 {
		opts.Concurrency = 3
	}

	ctx, cancel := context.WithCancel(context.Background())

	a.mu.Lock()
	if a.doc == nil {
		a.mu.Unlock()
		cancel()
		return TranslationSummary{}, errors.New("请先加载 Localization 文件")
	}
	if a.translateCancel != nil {
		a.mu.Unlock()
		cancel()
		return TranslationSummary{}, errors.New("已有翻译任务正在运行")
	}
	a.translateCancel = cancel
	a.paused = false
	rows := a.collectRowsLocked(opts.Overwrite)
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		if a.translateCancel != nil {
			a.translateCancel = nil
		}
		a.paused = false
		a.mu.Unlock()
		cancel()
	}()

	summary := TranslationSummary{Total: len(rows)}
	stopMessage := ""
	if len(rows) == 0 {
		summary.Message = "没有需要翻译的空白条目"
		a.emitProgress(summary, "done", nil, nil)
		return summary, nil
	}
	batches := buildTranslationBatches(rows, opts.BatchSize)

	a.emitProgress(summary, "running", nil, nil)

	taskCh := make(chan translationTask)
	resultCh := make(chan translationResult)
	var wg sync.WaitGroup

	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				if err := a.waitIfPaused(ctx); err != nil {
					return
				}
				results := translateRows(ctx, opts, task.rows)
				for _, result := range results {
					select {
					case <-ctx.Done():
						return
					case resultCh <- result:
					}
				}
			}
		}()
	}

	go func() {
		defer close(taskCh)
		for _, batch := range batches {
			if len(batch) == 0 {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case taskCh <- translationTask{rows: batch}:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for result := range resultCh {
		if result.err != nil {
			summary.Failed += len(result.rows)
			a.emitProgress(summary, "running", nil, result.rows)
			runtime.EventsEmit(a.ctx, "translation:error", result.err.Error())
			continue
		}

		updatedRows, err := a.applyTranslations(result.rows, result.translations)
		if err != nil {
			summary.Failed += len(result.rows)
			runtime.EventsEmit(a.ctx, "translation:error", err.Error())
			a.emitProgress(summary, "running", nil, result.rows)
			if isLocalizationSaveError(err) {
				stopMessage = err.Error()
				cancel()
			}
		} else {
			summary.Completed += len(updatedRows)
			a.emitProgress(summary, "running", updatedRows, nil)
		}
	}

	if ctx.Err() != nil {
		summary.Canceled = true
		if stopMessage != "" {
			summary.Message = stopMessage
		} else {
			summary.Message = "任务已终止"
		}
		a.emitProgress(summary, "canceled", nil, nil)
		return summary, nil
	}

	if summary.Failed > 0 {
		summary.Message = fmt.Sprintf("完成 %d 条，失败 %d 条；失败条目保持空白，可再次运行续传", summary.Completed, summary.Failed)
	} else {
		summary.Message = fmt.Sprintf("已完成 %d 条翻译", summary.Completed)
	}
	a.emitProgress(summary, "done", nil, nil)
	return summary, nil
}

func (a *App) SetPaused(paused bool) {
	a.mu.Lock()
	a.paused = paused
	a.mu.Unlock()
	state := "running"
	if paused {
		state = "paused"
	}
	runtime.EventsEmit(a.ctx, "translation:state", state)
}

func (a *App) StopTranslation() {
	a.mu.Lock()
	cancel := a.translateCancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) collectRowsLocked(overwrite bool) []LocalizationRow {
	var rows []LocalizationRow
	for _, row := range a.doc.Rows {
		if !row.Translatable {
			continue
		}
		if !overwrite && hasUsableExistingTranslation(row) {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func (a *App) applyTranslations(rows []LocalizationRow, translations []string) ([]LocalizationRow, error) {
	if len(rows) != len(translations) {
		return nil, fmt.Errorf("AI 返回数量不一致: 输入 %d 条，返回 %d 条", len(rows), len(translations))
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.doc == nil {
		return nil, errors.New("Localization 文件状态不存在")
	}

	originalRecords := cloneRecords(a.doc.records)
	for i, row := range rows {
		if row.Index <= a.doc.headerRowIndex || row.Index >= len(a.doc.records) {
			continue
		}
		a.doc.records[row.Index] = padRecord(a.doc.records[row.Index], len(a.doc.Header))
		a.doc.records[row.Index][a.doc.SchineseIndex] = translations[i]
	}
	a.rebuildRowsLocked()

	if err := saveDocument(a.doc); err != nil {
		a.doc.records = originalRecords
		a.rebuildRowsLocked()
		return nil, err
	}

	updated := make([]LocalizationRow, 0, len(rows))
	rowSet := map[int]struct{}{}
	for _, row := range rows {
		rowSet[row.Index] = struct{}{}
	}
	for _, row := range a.doc.Rows {
		if _, ok := rowSet[row.Index]; ok {
			updated = append(updated, row)
		}
	}
	return updated, nil
}

func (a *App) waitIfPaused(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		paused := a.paused
		a.mu.Unlock()
		if !paused {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *App) emitProgress(summary TranslationSummary, status string, rows []LocalizationRow, failedRows []LocalizationRow) {
	runtime.EventsEmit(a.ctx, "translation:progress", map[string]interface{}{
		"total":      summary.Total,
		"completed":  summary.Completed,
		"failed":     summary.Failed,
		"status":     status,
		"rows":       rows,
		"failedRows": failedRows,
	})
}

func clamp(value int, min int, max int) int {
	if value == 0 {
		return 0
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
