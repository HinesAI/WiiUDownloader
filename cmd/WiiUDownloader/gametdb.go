package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

const (
	gameTDBZipURL      = "https://www.gametdb.com/wiiutdb.zip"
	gameTDBHTTPTimeout = 45 * time.Second
	gameTDBUserAgent   = appUserAgent
)

type gameTDBMeta struct {
	Year     string
	Platform string
}

type gameTDBIndex struct {
	byKey map[string]gameTDBMeta
}

var (
	gameTDBOnce   sync.Once
	gameTDBIdx    *gameTDBIndex
	gameTDBErr    error
	gameTDBClient = &http.Client{Timeout: gameTDBHTTPTimeout}
)

var (
	parenStripper = regexp.MustCompile(`\([^)]*\)`)
	spaceCollapse = regexp.MustCompile(`\s+`)
)

func lookupGameTDBMeta(entry wiiudownloader.TitleEntry) (gameTDBMeta, bool) {
	idx, err := ensureGameTDBIndex()
	if err != nil || idx == nil {
		return gameTDBMeta{}, false
	}

	keys := candidateKeysForTitle(entry)
	for _, key := range keys {
		if meta, ok := idx.byKey[key]; ok {
			return meta, true
		}
	}
	return gameTDBMeta{}, false
}

func ensureGameTDBIndex() (*gameTDBIndex, error) {
	gameTDBOnce.Do(func() {
		gameTDBIdx, gameTDBErr = loadOrFetchGameTDBIndex()
		if gameTDBErr != nil {
			log.Printf("GameTDB metadata unavailable: %v", gameTDBErr)
		}
	})
	return gameTDBIdx, gameTDBErr
}

func loadOrFetchGameTDBIndex() (*gameTDBIndex, error) {
	cachePath, err := gameTDBCachePath()
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		if idx, err := parseGameTDBXML(data); err == nil {
			return idx, nil
		}
	}

	data, err := downloadGameTDBXML()
	if err != nil {
		return nil, err
	}
	idx, err := parseGameTDBXML(data)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(cachePath, data, 0o644)
	return idx, nil
}

func gameTDBCachePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, "WiiUDownloader")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "wiiutdb.xml"), nil
}

func downloadGameTDBXML() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, gameTDBZipURL, nil)
	if err != nil {
		return nil, err
	}
	// GameTDB returns 403 for some query strings / default Go user-agents.
	req.Header.Set("User-Agent", gameTDBUserAgent)
	req.Header.Set("Accept", "application/zip,application/octet-stream,*/*")

	resp, err := gameTDBClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GameTDB download status %d", resp.StatusCode)
	}
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("no xml found in GameTDB zip")
}

type gameTDBDatafile struct {
	Games []gameTDBGame `xml:"game"`
}

type gameTDBGame struct {
	Name      string          `xml:"name,attr"`
	Type      string          `xml:"type"`
	Region    string          `xml:"region"`
	Locales   []gameTDBLocale `xml:"locale"`
	Date      gameTDBDate     `xml:"date"`
	Publisher string          `xml:"publisher"`
}

type gameTDBLocale struct {
	Lang  string `xml:"lang,attr"`
	Title string `xml:"title"`
}

type gameTDBDate struct {
	Year  string `xml:"year,attr"`
	Month string `xml:"month,attr"`
	Day   string `xml:"day,attr"`
}

func parseGameTDBXML(data []byte) (*gameTDBIndex, error) {
	var doc gameTDBDatafile
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	idx := &gameTDBIndex{byKey: make(map[string]gameTDBMeta, len(doc.Games)*3)}
	for _, game := range doc.Games {
		meta := gameTDBMeta{
			Year:     strings.TrimSpace(game.Date.Year),
			Platform: formatGameTDBPlatform(game.Type),
		}
		if meta.Year == "" && meta.Platform == "" {
			continue
		}

		regionHint := normalizeGameTDBRegion(game.Region)
		addKey := func(raw string) {
			key := normalizeTitleKey(raw)
			if key == "" {
				return
			}
			storeGameTDBMeta(idx.byKey, key, meta)
			if regionHint != "" {
				storeGameTDBMeta(idx.byKey, key+"|"+regionHint, meta)
			}
		}

		addKey(game.Name)
		for _, loc := range game.Locales {
			addKey(loc.Title)
		}
	}
	return idx, nil
}

func storeGameTDBMeta(dst map[string]gameTDBMeta, key string, meta gameTDBMeta) {
	existing, ok := dst[key]
	if !ok {
		dst[key] = meta
		return
	}
	// Prefer entries that include both year and platform.
	existingScore := metaScore(existing)
	newScore := metaScore(meta)
	if newScore > existingScore {
		dst[key] = meta
	}
}

func metaScore(m gameTDBMeta) int {
	score := 0
	if m.Year != "" {
		score += 2
	}
	if m.Platform != "" {
		score++
	}
	return score
}

func formatGameTDBPlatform(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "WIIU":
		return "Wii U"
	case "ESHOP":
		return "Wii U (eShop)"
	case "VC-NES":
		return "NES"
	case "VC-SNES":
		return "SNES"
	case "VC-N64":
		return "N64"
	case "VC-GBA":
		return "GBA"
	case "VC-DS":
		return "Nintendo DS"
	case "VC-PCE":
		return "TurboGrafx-16"
	case "VC-MSX":
		return "MSX"
	case "CHANNEL":
		return "Channel"
	default:
		if t == "" {
			return ""
		}
		return t
	}
}

func normalizeGameTDBRegion(region string) string {
	switch strings.ToUpper(strings.TrimSpace(region)) {
	case "NTSC-U", "USA", "US":
		return "usa"
	case "NTSC-J", "JPN", "JAPAN", "JA":
		return "japan"
	case "PAL", "EUR", "EUROPE", "EN":
		return "europe"
	default:
		return ""
	}
}

func candidateKeysForTitle(entry wiiudownloader.TitleEntry) []string {
	base := normalizeTitleKey(entry.Name)
	if base == "" {
		return nil
	}
	keys := []string{base}

	switch {
	case entry.Region&wiiudownloader.MCP_REGION_USA != 0:
		keys = append(keys, base+"|usa")
	case entry.Region&wiiudownloader.MCP_REGION_EUROPE != 0:
		keys = append(keys, base+"|europe")
	case entry.Region&wiiudownloader.MCP_REGION_JAPAN != 0:
		keys = append(keys, base+"|japan")
	}

	// Also try stripping common trailing " - something" subtitles.
	if i := strings.Index(base, " - "); i > 0 {
		keys = append(keys, base[:i])
	}
	return keys
}

func normalizeTitleKey(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Prefer English title inside Japanese "日本語 (English)" style names.
	if open := strings.LastIndex(name, "("); open >= 0 {
		if close := strings.LastIndex(name, ")"); close > open {
			inner := strings.TrimSpace(name[open+1 : close])
			if looksMostlyLatin(inner) && len(inner) >= 3 {
				name = inner
			}
		}
	}

	name = parenStripper.ReplaceAllString(name, " ")
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		if unicode.IsSpace(r) || r == '-' || r == '\'' {
			return ' '
		}
		return -1
	}, name)
	name = spaceCollapse.ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

func looksMostlyLatin(s string) bool {
	letters := 0
	latin := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if r <= unicode.MaxASCII {
				latin++
			}
		}
	}
	return letters > 0 && latin*2 >= letters
}
