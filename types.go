package main

import (
	"context"
	"sync"
)

type App struct {
	ctx context.Context

	mu              sync.Mutex
	doc             *LocalizationDocument
	translateCancel context.CancelFunc
	paused          bool
}

type LocalizationDocument struct {
	Path             string            `json:"path"`
	Delimiter        string            `json:"delimiter"`
	Header           []string          `json:"header"`
	Rows             []LocalizationRow `json:"rows"`
	EnglishIndex     int               `json:"englishIndex"`
	SchineseIndex    int               `json:"schineseIndex"`
	NoTranslateIndex int               `json:"noTranslateIndex"`
	TotalRows        int               `json:"totalRows"`
	PendingRows      int               `json:"pendingRows"`

	headerRowIndex int
	records        [][]string
	delimiterRune  rune
}

type LocalizationRow struct {
	Index        int      `json:"index"`
	RowNumber    int      `json:"rowNumber"`
	Key          string   `json:"key"`
	English      string   `json:"english"`
	Schinese     string   `json:"schinese"`
	NoTranslate  string   `json:"noTranslate"`
	Translatable bool     `json:"translatable"`
	Cells        []string `json:"cells"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	Owner   string `json:"owner,omitempty"`
	Created int64  `json:"created,omitempty"`
}

type TranslateOptions struct {
	BaseURL     string `json:"baseUrl"`
	APIKey      string `json:"apiKey"`
	Model       string `json:"model"`
	BatchSize   int    `json:"batchSize"`
	Concurrency int    `json:"concurrency"`
	Overwrite   bool   `json:"overwrite"`
}

type TranslationSummary struct {
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Canceled  bool   `json:"canceled"`
	Message   string `json:"message"`
}

type translationTask struct {
	rows []LocalizationRow
}

type translationResult struct {
	rows         []LocalizationRow
	translations []string
	err          error
}

type translationRequestItem struct {
	ID   int    `json:"id"`
	Key  string `json:"key,omitempty"`
	Text string `json:"text"`
}

type translationResponseItem struct {
	ID          int    `json:"id"`
	Translation string `json:"translation"`
	Text        string `json:"text,omitempty"`
	Schinese    string `json:"schinese,omitempty"`
	Chinese     string `json:"chinese,omitempty"`
	Translated  string `json:"translated,omitempty"`
	Unchanged   bool   `json:"unchanged,omitempty"`
}
