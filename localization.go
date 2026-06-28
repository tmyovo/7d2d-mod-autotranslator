package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) LoadLocalization(path string) (*LocalizationDocument, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("路径不能为空")
	}

	filePath, err := findLocalizationFile(path)
	if err != nil {
		return nil, err
	}

	doc, err := parseLocalizationFile(filePath)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.doc = doc
	a.mu.Unlock()

	return cloneDocument(doc), nil
}

func (a *App) SaveRow(rowIndex int, schinese string) (LocalizationRow, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.doc == nil {
		return LocalizationRow{}, errors.New("请先加载 Localization 文件")
	}
	if rowIndex <= a.doc.headerRowIndex || rowIndex >= len(a.doc.records) {
		return LocalizationRow{}, fmt.Errorf("无效行索引: %d", rowIndex)
	}

	originalRecords := cloneRecords(a.doc.records)
	a.doc.records[rowIndex] = padRecord(a.doc.records[rowIndex], len(a.doc.Header))
	a.doc.records[rowIndex][a.doc.SchineseIndex] = schinese
	a.rebuildRowsLocked()

	if err := saveDocument(a.doc); err != nil {
		a.doc.records = originalRecords
		a.rebuildRowsLocked()
		return LocalizationRow{}, err
	}

	for _, row := range a.doc.Rows {
		if row.Index == rowIndex {
			return row, nil
		}
	}
	return LocalizationRow{}, errors.New("保存成功，但未找到更新后的行")
}

func (a *App) rowByIndexLocked(rowIndex int) (LocalizationRow, error) {
	if rowIndex <= a.doc.headerRowIndex || rowIndex >= len(a.doc.records) {
		return LocalizationRow{}, fmt.Errorf("无效行索引: %d", rowIndex)
	}
	for _, row := range a.doc.Rows {
		if row.Index == rowIndex {
			return row, nil
		}
	}
	return LocalizationRow{}, errors.New("未找到指定行")
}

func (a *App) rebuildRowsLocked() {
	rows, pending := buildRows(a.doc)
	a.doc.Rows = rows
	a.doc.TotalRows = len(rows)
	a.doc.PendingRows = pending
}

func findLocalizationFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		if isLocalizationName(path) {
			return path, nil
		}
		return "", errors.New("请选择 Localization.csv / Localization.txt 文件，或包含 Config 目录的 Mod 文件夹")
	}

	candidates := []string{
		filepath.Join(path, "Config", "Localization.csv"),
		filepath.Join(path, "Config", "Localization.txt"),
		filepath.Join(path, "Localization.csv"),
		filepath.Join(path, "Localization.txt"),
	}
	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
	}

	configDir := filepath.Join(path, "Config")
	if stat, err := os.Stat(configDir); err == nil && stat.IsDir() {
		var found string
		_ = filepath.WalkDir(configDir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if isLocalizationName(p) {
				found = p
				return filepath.SkipAll
			}
			return nil
		})
		if found != "" {
			return found, nil
		}
	}

	return "", errors.New("未找到 Config/Localization.csv 或 Config/Localization.txt")
}

func isLocalizationName(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return name == "localization.csv" || name == "localization.txt"
}

func parseLocalizationFile(path string) (*LocalizationDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	delimiter := detectDelimiter(data)

	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败: %w", err)
	}
	if len(records) == 0 {
		return nil, errors.New("Localization 文件为空")
	}

	headerIndex, englishIndex, schineseIndex, noTranslateIndex := findHeader(records)
	if headerIndex < 0 || englishIndex < 0 {
		return nil, errors.New("未找到 english/English 表头")
	}

	header := append([]string(nil), records[headerIndex]...)
	if schineseIndex < 0 {
		header = append(header, "schinese")
		schineseIndex = len(header) - 1
		records[headerIndex] = header
	}

	for i := range records {
		records[i] = padRecord(records[i], len(header))
	}

	doc := &LocalizationDocument{
		Path:             path,
		Delimiter:        string(delimiter),
		Header:           header,
		EnglishIndex:     englishIndex,
		SchineseIndex:    schineseIndex,
		NoTranslateIndex: noTranslateIndex,
		headerRowIndex:   headerIndex,
		records:          records,
		delimiterRune:    delimiter,
	}
	rows, pending := buildRows(doc)
	doc.Rows = rows
	doc.TotalRows = len(rows)
	doc.PendingRows = pending
	return doc, nil
}

func detectDelimiter(data []byte) rune {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tabCount := strings.Count(line, "\t")
		commaCount := strings.Count(line, ",")
		semicolonCount := strings.Count(line, ";")
		if tabCount > commaCount && tabCount > semicolonCount {
			return '\t'
		}
		if semicolonCount > commaCount {
			return ';'
		}
		return ','
	}
	return ','
}

func findHeader(records [][]string) (headerIndex int, englishIndex int, schineseIndex int, noTranslateIndex int) {
	for rowIndex, record := range records {
		ei, si, ni := -1, -1, -1
		for i, col := range record {
			name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(col, "\uFEFF")))
			switch name {
			case "english":
				ei = i
			case "schinese":
				si = i
			case "notranslate", "no_translate", "no translate":
				ni = i
			}
		}
		if ei >= 0 {
			return rowIndex, ei, si, ni
		}
	}
	return -1, -1, -1, -1
}

func buildRows(doc *LocalizationDocument) ([]LocalizationRow, int) {
	keyIndex := findColumn(doc.Header, "key")
	rows := make([]LocalizationRow, 0, len(doc.records))
	pending := 0
	for i := doc.headerRowIndex + 1; i < len(doc.records); i++ {
		record := padRecord(doc.records[i], len(doc.Header))
		english := record[doc.EnglishIndex]
		schinese := record[doc.SchineseIndex]
		noTranslate := ""
		if doc.NoTranslateIndex >= 0 && doc.NoTranslateIndex < len(record) {
			noTranslate = record[doc.NoTranslateIndex]
		}
		key := ""
		if keyIndex >= 0 && keyIndex < len(record) {
			key = record[keyIndex]
		}
		translatable := strings.TrimSpace(english) != "" && !isNoTranslate(noTranslate)
		if translatable && strings.TrimSpace(schinese) == "" {
			pending++
		}
		rows = append(rows, LocalizationRow{
			Index:        i,
			RowNumber:    i + 1,
			Key:          key,
			English:      english,
			Schinese:     schinese,
			NoTranslate:  noTranslate,
			Translatable: translatable,
			Cells:        append([]string(nil), record...),
		})
	}
	return rows, pending
}

func saveDocument(doc *LocalizationDocument) error {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(&buf)
	writer.Comma = doc.delimiterRune
	writer.UseCRLF = true
	if err := writer.WriteAll(doc.records); err != nil {
		return err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}

	if err := os.WriteFile(doc.Path, buf.Bytes(), 0644); err != nil {
		return newLocalizationSaveError(doc.Path, err)
	}
	return nil
}

type localizationSaveError struct {
	path     string
	err      error
	fileBusy bool
}

func newLocalizationSaveError(path string, err error) error {
	return &localizationSaveError{
		path:     path,
		err:      err,
		fileBusy: isFileBusyError(err),
	}
}

func (e *localizationSaveError) Error() string {
	if e.fileBusy {
		return fmt.Sprintf("Localization 文件被其他程序占用，无法保存。\n请关闭 Excel、记事本或其他正在打开该文件的程序，然后重试。\n文件：%s\n原始错误：%v", e.path, e.err)
	}
	return fmt.Sprintf("保存 Localization 文件失败。\n文件：%s\n原始错误：%v", e.path, e.err)
}

func (e *localizationSaveError) Unwrap() error {
	return e.err
}

func isLocalizationSaveError(err error) bool {
	var saveErr *localizationSaveError
	return errors.As(err, &saveErr)
}

func isFileBusyError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "being used by another process") ||
		strings.Contains(message, "process cannot access the file") ||
		strings.Contains(message, "sharing violation") ||
		strings.Contains(message, "lock violation") ||
		strings.Contains(message, "文件正由另一进程使用") ||
		strings.Contains(message, "另一个程序正在使用此文件") ||
		strings.Contains(message, "另一个进程正在使用此文件") ||
		strings.Contains(message, "该文件正由另一个进程使用")
}

func cloneDocument(doc *LocalizationDocument) *LocalizationDocument {
	clone := *doc
	clone.Header = append([]string(nil), doc.Header...)
	clone.Rows = append([]LocalizationRow(nil), doc.Rows...)
	return &clone
}

func cloneRecords(records [][]string) [][]string {
	clone := make([][]string, len(records))
	for i := range records {
		clone[i] = append([]string(nil), records[i]...)
	}
	return clone
}

func padRecord(record []string, length int) []string {
	if len(record) >= length {
		return record
	}
	padded := make([]string, length)
	copy(padded, record)
	return padded
}

func findColumn(header []string, name string) int {
	name = strings.ToLower(name)
	for i, col := range header {
		if strings.ToLower(strings.TrimSpace(col)) == name {
			return i
		}
	}
	return -1
}

func isNoTranslate(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	return value != "0" && value != "false" && value != "no"
}
