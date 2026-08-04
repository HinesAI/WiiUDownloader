package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	idbeIconDataBaseURL = "https://idbe-wup.cdn.nintendo.net/icondata"
	idbeHeaderSize      = 2
	idbeSHAOffset       = 0x00
	idbeSHASize         = 0x20
	idbeMagicOffset     = 0x40
	idbeTGAOffset       = 0x2050
	idbeHTTPTimeout     = 20 * time.Second
)

var (
	idbeAESKeys = [4][]byte{
		{0x4A, 0xB9, 0xA4, 0x0E, 0x14, 0x69, 0x75, 0xA8, 0x4B, 0xB1, 0xB4, 0xF3, 0xEC, 0xEF, 0xC4, 0x7B},
		{0x90, 0xA0, 0xBB, 0x1E, 0x0E, 0x86, 0x4A, 0xE8, 0x7D, 0x13, 0xA6, 0xA0, 0x3D, 0x28, 0xC9, 0xB8},
		{0xFF, 0xBB, 0x57, 0xC1, 0x4E, 0x98, 0xEC, 0x69, 0x75, 0xB3, 0x84, 0xFC, 0xF4, 0x07, 0x86, 0xB5},
		{0x80, 0x92, 0x37, 0x99, 0xB4, 0x1F, 0x36, 0xA6, 0xA7, 0x5F, 0xB8, 0xB4, 0x8C, 0x95, 0xF6, 0x6F},
	}
	idbeAESIV = []byte{0xA4, 0x69, 0x87, 0xAE, 0x47, 0xD8, 0x2B, 0xB4, 0xFA, 0x8A, 0xBC, 0x04, 0x50, 0x28, 0x5F, 0xA4}

	idbeHTTPClient = &http.Client{
		Timeout: idbeHTTPTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				// Nintendo's CDN uses a private CA that is not in system trust stores.
				InsecureSkipVerify: true,
			},
		},
	}

	idbeCacheMu sync.Mutex
)

func coverCachePath(titleID uint64) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheDir, "WiiUDownloader", "covers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%016x.png", titleID)), nil
}

func loadCachedCoverPNG(titleID uint64) ([]byte, bool) {
	path, err := coverCachePath(titleID)
	if err != nil {
		return nil, false
	}
	idbeCacheMu.Lock()
	defer idbeCacheMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func saveCachedCoverPNG(titleID uint64, pngData []byte) {
	path, err := coverCachePath(titleID)
	if err != nil {
		return
	}
	idbeCacheMu.Lock()
	defer idbeCacheMu.Unlock()
	_ = os.WriteFile(path, pngData, 0o644)
}

func fetchTitleCoverPNG(titleID uint64) ([]byte, error) {
	if cached, ok := loadCachedCoverPNG(titleID); ok {
		return cached, nil
	}

	keyBucket := uint8((titleID >> 8) & 0xFF)
	url := fmt.Sprintf("%s/%02X/%016X.idbe", idbeIconDataBaseURL, keyBucket, titleID)
	resp, err := idbeHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("idbe fetch failed: status %d", resp.StatusCode)
	}

	encrypted, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	pngData, err := idbeToPNG(encrypted)
	if err != nil {
		return nil, err
	}
	saveCachedCoverPNG(titleID, pngData)
	return pngData, nil
}

func idbeToPNG(encrypted []byte) ([]byte, error) {
	plain, err := decryptIDBE(encrypted)
	if err != nil {
		return nil, err
	}
	if len(plain) <= idbeTGAOffset {
		return nil, fmt.Errorf("idbe payload too short")
	}
	img, err := decodeUncompressedTGA(plain[idbeTGAOffset:])
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decryptIDBE(data []byte) ([]byte, error) {
	if len(data) < idbeHeaderSize+aes.BlockSize {
		return nil, fmt.Errorf("idbe data too short")
	}
	keyIndex := int(data[1])
	if keyIndex < 0 || keyIndex >= len(idbeAESKeys) {
		return nil, fmt.Errorf("invalid idbe key index %d", keyIndex)
	}

	encrypted := data[idbeHeaderSize:]
	if len(encrypted)%aes.BlockSize != 0 {
		encrypted = encrypted[:len(encrypted)-len(encrypted)%aes.BlockSize]
	}
	if len(encrypted) == 0 {
		return nil, fmt.Errorf("idbe encrypted payload empty")
	}

	block, err := aes.NewCipher(idbeAESKeys[keyIndex])
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, idbeAESIV).CryptBlocks(plain, encrypted)

	if len(plain) < idbeSHASize+idbeMagicOffset {
		return nil, fmt.Errorf("decrypted idbe too short")
	}
	sum := sha256.Sum256(plain[idbeSHASize:])
	if !bytes.Equal(sum[:], plain[idbeSHAOffset:idbeSHAOffset+idbeSHASize]) {
		return nil, fmt.Errorf("idbe checksum mismatch")
	}
	if !bytes.Equal(plain[idbeMagicOffset:idbeMagicOffset+4], []byte{0xC0, 0xC0, 0xC0, 0xC0}) {
		return nil, fmt.Errorf("idbe magic mismatch")
	}
	return plain, nil
}

func decodeUncompressedTGA(data []byte) (image.Image, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("tga header too short")
	}
	idLength := int(data[0])
	imageType := data[2]
	if imageType != 2 {
		return nil, fmt.Errorf("unsupported tga type %d", imageType)
	}
	width := int(binary.LittleEndian.Uint16(data[12:14]))
	height := int(binary.LittleEndian.Uint16(data[14:16]))
	bpp := int(data[16])
	descriptor := data[17]
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid tga dimensions")
	}
	if bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("unsupported tga bpp %d", bpp)
	}

	bytesPerPixel := bpp / 8
	pixelDataOffset := 18 + idLength
	needed := pixelDataOffset + width*height*bytesPerPixel
	if len(data) < needed {
		return nil, fmt.Errorf("tga pixel data truncated")
	}

	topOrigin := descriptor&0x20 != 0
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	src := data[pixelDataOffset:]

	for y := 0; y < height; y++ {
		srcY := y
		if !topOrigin {
			srcY = height - 1 - y
		}
		for x := 0; x < width; x++ {
			i := (srcY*width + x) * bytesPerPixel
			dst := (y*width + x) * 4
			img.Pix[dst+0] = src[i+2] // R
			img.Pix[dst+1] = src[i+1] // G
			img.Pix[dst+2] = src[i+0] // B
			if bytesPerPixel == 4 {
				img.Pix[dst+3] = src[i+3]
			} else {
				img.Pix[dst+3] = 0xFF
			}
		}
	}
	return img, nil
}
