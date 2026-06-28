package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindAndParseLocalizationInConfigFolder(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "Config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	localizationPath := filepath.Join(configDir, "Localization.csv")
	content := "Key,File,Type,UsedInMainMenu,NoTranslate,english,schinese\nsample_key,items,Item,,,Hello,\n"
	if err := os.WriteFile(localizationPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	found, err := findLocalizationFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if found != localizationPath {
		t.Fatalf("expected %q, got %q", localizationPath, found)
	}

	doc, err := parseLocalizationFile(found)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Path != localizationPath {
		t.Fatalf("expected document path %q, got %q", localizationPath, doc.Path)
	}
	if doc.EnglishIndex != 5 {
		t.Fatalf("expected english column index 5, got %d", doc.EnglishIndex)
	}
	if doc.SchineseIndex != 6 {
		t.Fatalf("expected schinese column index 6, got %d", doc.SchineseIndex)
	}
	if len(doc.Rows) != 1 || doc.Rows[0].English != "Hello" {
		t.Fatalf("unexpected rows: %#v", doc.Rows)
	}
}

func TestHasUsableExistingTranslationRejectsEnglishOnlySchinese(t *testing.T) {
	row := LocalizationRow{
		English:  "Kill the Electric Demon",
		Schinese: "Kill the Electric Demon",
	}
	if hasUsableExistingTranslation(row) {
		t.Fatal("english-only schinese should be treated as untranslated")
	}

	row.Schinese = "击杀电魔"
	if !hasUsableExistingTranslation(row) {
		t.Fatal("chinese schinese should be treated as translated")
	}

	acronymOnly := LocalizationRow{
		English:  "AEC",
		Schinese: "AEC",
	}
	if !hasUsableExistingTranslation(acronymOnly) {
		t.Fatal("acronym-only values should not require Chinese characters")
	}
}

func TestIsNoTranslateRespectsNonEmptyModMarkers(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "yes", "y", "skip", "no_translate", "notranslate", "2", "x", "item"}
	for _, value := range truthy {
		if !isNoTranslate(value) {
			t.Fatalf("expected %q to be treated as NoTranslate", value)
		}
	}

	falsy := []string{"", "0", "false", "FALSE", "no", "NO"}
	for _, value := range falsy {
		if isNoTranslate(value) {
			t.Fatalf("expected %q to remain translatable", value)
		}
	}
}

func TestBuildTranslationBatchesUsesSingleItemForQuestRows(t *testing.T) {
	rows := []LocalizationRow{
		{Key: "itemDesc", English: "This is a normal item description without command-style text."},
		{Key: "questExample_obj_kill", English: "Kill the incoming infected"},
		{Key: "itemName", English: "Electric Component"},
	}

	batches := buildTranslationBatches(rows, 10)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d: %#v", len(batches), batches)
	}
	if len(batches[1]) != 1 || batches[1][0].Key != "questExample_obj_kill" {
		t.Fatalf("quest row should be isolated, got %#v", batches[1])
	}
}

func TestBuildTranslationBatchesFlushesRemainderBelowBatchSize(t *testing.T) {
	rows := make([]LocalizationRow, 48)
	for i := range rows {
		rows[i] = LocalizationRow{
			Key:     "itemName",
			English: "Electric Component",
		}
	}

	batches := buildTranslationBatches(rows, 50)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if len(batches[0]) != 48 {
		t.Fatalf("expected final partial batch with 48 rows, got %d", len(batches[0]))
	}
}

func TestParseTranslationObjectItemsRequiresMatchingIDs(t *testing.T) {
	rows := []LocalizationRow{
		{Index: 10, RowNumber: 11, English: "Kill the Electric Demon"},
	}

	items, err := parseTranslationObjectItems(`[{"id":10,"translation":"击杀电魔"}]`, rows)
	if err != nil {
		t.Fatal(err)
	}
	translations := translationsFromItems(items)
	if len(translations) != 1 || translations[0] != "击杀电魔" {
		t.Fatalf("unexpected translations: %#v", translations)
	}

	if _, err := parseTranslationObjectItems(`[{"id":11,"translation":"击杀电魔"}]`, rows); err == nil {
		t.Fatal("expected mismatched id to fail")
	}
}

func TestValidateTranslationRequiresChineseWhenSourceNeedsTranslation(t *testing.T) {
	if err := validateTranslationResult("Survive wave 3", "Survive wave 3", false); err == nil {
		t.Fatal("english-only output should fail")
	}
	if err := validateTranslationResult("Survive wave 3", "存活第 3 波", false); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTranslationAllowsExplicitUnchangedText(t *testing.T) {
	if err := validateTranslationResult("discord.gg/CP4HDb3r3j", "discord.gg/CP4HDb3r3j", true); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTranslationRequiresUnchangedTextToMatchSource(t *testing.T) {
	err := validateTranslationResult("discord.gg/CP4HDb3r3j", "discord.gg/changed", true)
	if err == nil {
		t.Fatal("expected changed unchanged text to fail")
	}
}
