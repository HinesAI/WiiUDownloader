package main

import (
	"os"
	"testing"
)

func TestDecryptIDBESample(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.idbe")
	if err != nil {
		t.Skip("testdata/sample.idbe not present")
	}
	pngData, err := idbeToPNG(data)
	if err != nil {
		t.Fatalf("idbeToPNG: %v", err)
	}
	if len(pngData) < 100 || pngData[0] != 0x89 {
		t.Fatalf("expected PNG output, got %d bytes", len(pngData))
	}
}

func TestFetchTitleCoverPNG(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	// Mario Kart 8 USA
	pngData, err := fetchTitleCoverPNG(0x000500001010EC00)
	if err != nil {
		t.Fatalf("fetchTitleCoverPNG: %v", err)
	}
	if len(pngData) < 100 || pngData[0] != 0x89 {
		t.Fatalf("expected PNG output, got %d bytes", len(pngData))
	}
}
