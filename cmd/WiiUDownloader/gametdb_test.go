package main

import (
	"os"
	"testing"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

func TestParseGameTDBXMLMarioKart(t *testing.T) {
	data, err := os.ReadFile(mustGameTDBFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	idx, err := parseGameTDBXML(data)
	if err != nil {
		t.Fatal(err)
	}
	entry := wiiudownloader.TitleEntry{
		Name:   "Mario Kart 8",
		Region: wiiudownloader.MCP_REGION_USA,
	}
	meta, ok := idx.byKey[normalizeTitleKey(entry.Name)+"|usa"]
	if !ok {
		meta, ok = idx.byKey[normalizeTitleKey(entry.Name)]
	}
	if !ok {
		t.Fatalf("mario kart meta missing")
	}
	if meta.Year != "2014" {
		t.Fatalf("year=%q want 2014", meta.Year)
	}
	if meta.Platform == "" {
		t.Fatalf("expected platform")
	}
}

func TestNormalizeTitleKeyEnglishInParens(t *testing.T) {
	got := normalizeTitleKey("マリオカート８ (Mario Kart 8)")
	if got != "mario kart 8" {
		t.Fatalf("got %q", got)
	}
}

func mustGameTDBFixture(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.ExpandEnv("$HOME/Library/Caches/WiiUDownloader/wiiutdb.xml"),
		"/tmp/wiiu_db/wiiutdb.xml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("GameTDB fixture not present")
	return ""
}
