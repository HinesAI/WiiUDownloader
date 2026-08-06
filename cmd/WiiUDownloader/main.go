package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	NETWORK_DIAL_TIMEOUT         = 30 * time.Second
	NETWORK_DIAL_KEEP_ALIVE      = 30 * time.Second
	FALLBACK_DNS_DIAL_TIMEOUT    = 10 * time.Second
	HTTP_MAX_IDLE_CONNS          = 100
	HTTP_MAX_IDLE_CONNS_PER_HOST = 100
	HTTP_MAX_CONNS_PER_HOST      = 100
	HTTP_IDLE_CONN_TIMEOUT       = 90 * time.Second
	HTTP_TLS_HANDSHAKE_TIMEOUT   = 10 * time.Second
	HTTP_RESPONSE_HEADER_TIMEOUT = 10 * time.Second
	HTTP_EXPECT_CONTINUE_TIMEOUT = 1 * time.Second
	// Used only as DNS fallback when system resolver fails.
	FALLBACK_DNS_RESOLVER_ENDPOINT = "1.1.1.1:53"
)

func main() {
	runtime.LockOSThread()
	runtime.GOMAXPROCS(runtime.NumCPU())

	config, configErr := loadConfig()
	if config == nil {
		config = getDefaultConfig()
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if config.DarkMode {
			os.Setenv("GTK_THEME", "Adwaita:dark")
		} else {
			os.Setenv("GTK_THEME", "Adwaita")
		}
	}

	configureMacOSEnvironment()
	configureWindowsEnvironment()
	gtk.Init(nil)

	setDarkTheme(config.DarkMode)

	app, err := gtk.ApplicationNew("io.github.hinesai.wiiudownloaderfork", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		showFatalDialogAndLog("Error creating application", err)
		return
	}

	if runtime.GOOS == "darwin" {
		quitAction := glib.SimpleActionNew("quit", nil)
		quitAction.Connect("activate", func() {
			app.Quit()
		})
		app.AddAction(quitAction)
		app.SetAccelsForAction("app.quit", []string{"<Primary>q"})
	}

	client := buildHTTPClient()
	if configErr != nil {
		log.Printf("error loading config: %v", configErr)
		uiIdleAdd(func() {
			errorDialog := gtk.MessageDialogNew(nil, 0, gtk.MESSAGE_WARNING, gtk.BUTTONS_OK, "Error loading config: %v\n\nStarting with default settings.", configErr)
			errorDialog.Run()
			errorDialog.Destroy()
		})
	}

	win := NewMainWindow(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME), client, config)
	config.saveConfigCallback = func() {
		uiIdleAdd(func() {
			win.applyConfig(config)
		})
	}

	app.Connect("activate", func(app *gtk.Application) {
		if !config.DidInitialSetup {
			assistant, err := NewInitialSetupAssistantWindow(config)
			if err != nil {
				showFatalDialogAndLog("Error creating setup assistant", err)
				return
			}
			assistant.SetPostSetupCallback(func() {
				showMainWindow(app, win)
			})
			app.AddWindow(assistant.assistantWindow)
			assistant.assistantWindow.ShowAll()
			if win.window != nil {
				win.window.Hide()
			}
			return
		}

		showMainWindow(app, win)
	})

	app.Run(os.Args)
}

// configureWindowsEnvironment points GTK at the portable layout next to the
// .exe (DLLs, share/, gdk-pixbuf loaders). Without this, gtk.Init fails and
// a -H=windowsgui build exits with no visible error.
func configureWindowsEnvironment() {
	if runtime.GOOS != "windows" {
		return
	}
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("error getting executable path: %v", err)
		return
	}
	root := filepath.Dir(execPath)
	share := filepath.Join(root, "share")
	loaders := filepath.Join(root, "lib", "gdk-pixbuf-2.0", "2.10.0", "loaders")
	cache := filepath.Join(root, "lib", "gdk-pixbuf-2.0", "2.10.0", "loaders.cache")
	schemas := filepath.Join(root, "share", "glib-2.0", "schemas")

	os.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.Setenv("GTK_EXE_PREFIX", root)
	os.Setenv("GTK_DATA_PREFIX", root)
	os.Setenv("GSETTINGS_SCHEMA_DIR", schemas)
	os.Setenv("GDK_PIXBUF_MODULEDIR", loaders)
	os.Setenv("XDG_DATA_DIRS", share)

	if rewritten, err := rewriteWindowsLoadersCache(root, loaders, cache); err != nil {
		log.Printf("loaders.cache rewrite failed: %v", err)
		if _, statErr := os.Stat(cache); statErr == nil {
			os.Setenv("GDK_PIXBUF_MODULE_FILE", cache)
		}
	} else {
		os.Setenv("GDK_PIXBUF_MODULE_FILE", rewritten)
	}
}

// rewriteWindowsLoadersCache writes a runtime cache with absolute module paths.
// Bundled caches often list bare filenames, which resolve next to the exe and
// miss lib/gdk-pixbuf-2.0/.../loaders (and then SVG icons abort GTK).
func rewriteWindowsLoadersCache(root, loadersDir, bundledCache string) (string, error) {
	entries, err := os.ReadDir(loadersDir)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("# GdkPixbuf Image Loader Modules file\n#\n")

	// Prefer regenerating from the bundled cache text when present so we keep
	// mime/extension sniffs; only rewrite the module path lines.
	if data, err := os.ReadFile(bundledCache); err == nil {
		lines := strings.Split(string(data), "\n")
		for i := 0; i < len(lines); i++ {
			line := lines[i]
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, ".dll\"") {
				base := filepath.Base(strings.Trim(trimmed, "\""))
				abs := filepath.Join(loadersDir, base)
				if _, err := os.Stat(abs); err != nil {
					// Skip missing loaders (and their following metadata block).
					for i+1 < len(lines) {
						next := strings.TrimSpace(lines[i+1])
						if strings.HasPrefix(next, "\"") && strings.HasSuffix(next, ".dll\"") {
							break
						}
						if next == "" && i+2 < len(lines) {
							n2 := strings.TrimSpace(lines[i+2])
							if strings.HasPrefix(n2, "\"") && strings.HasSuffix(n2, ".dll\"") {
								break
							}
						}
						i++
						if next == "" {
							break
						}
					}
					continue
				}
				// gdk-pixbuf accepts forward slashes on Windows.
				b.WriteString("\"")
				b.WriteString(filepath.ToSlash(abs))
				b.WriteString("\"\n")
				continue
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "libpixbufloader-") || !strings.HasSuffix(name, ".dll") {
				continue
			}
			abs := filepath.Join(loadersDir, name)
			b.WriteString("\"")
			b.WriteString(filepath.ToSlash(abs))
			b.WriteString("\"\n")
		}
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = root
	}
	outDir := filepath.Join(cacheDir, "WiiUDownloader")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(outDir, "loaders.cache")
	tmp, err := os.CreateTemp(outDir, "loaders.cache.*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		_ = os.Remove(outPath)
		if err2 := os.Rename(tmpName, outPath); err2 != nil {
			return "", err2
		}
	}
	return outPath, nil
}

func configureMacOSEnvironment() {
	if runtime.GOOS != "darwin" {
		return
	}
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("error getting executable path: %v", err)
		return
	}
	if filepath.Base(filepath.Dir(execPath)) != "MacOS" {
		return
	}

	bundlePath := filepath.Dir(filepath.Dir(execPath))
	os.Unsetenv("DYLD_LIBRARY_PATH")
	os.Unsetenv("DYLD_FALLBACK_LIBRARY_PATH")
	os.Unsetenv("DYLD_INSERT_LIBRARIES")
	os.Unsetenv("PKG_CONFIG_PATH")

	glibPath := filepath.Join(bundlePath, "Resources", "share", "glib-2.0", "schemas")
	if _, err := os.Stat(glibPath); err == nil {
		os.Setenv("GSETTINGS_SCHEMA_DIR", glibPath)
	}

	loaderDir := filepath.Join(bundlePath, "MacOS", "lib", "gdkpixbuf_loaders")
	if _, err := os.Stat(loaderDir); err == nil {
		os.Setenv("GDK_PIXBUF_MODULE_DIR", loaderDir)
		if cacheOrig, ok := bundledLoadersCachePath(bundlePath); ok {
			if cachePath, err := rewriteLoadersCachePaths(cacheOrig, loaderDir); err == nil {
				os.Setenv("GDK_PIXBUF_MODULE_FILE", cachePath)
				log.Printf("Set GDK_PIXBUF_MODULE_FILE to rewritten bundle cache %s", cachePath)
			} else {
				log.Printf("loaders.cache rewrite failed: %v; falling back to runtime generation from %s", err, loaderDir)
				regenerateLoadersCache(loaderDir)
			}
		} else {
			log.Printf("Bundled loaders cache not found, falling back to runtime generation from %s", loaderDir)
			regenerateLoadersCache(loaderDir)
		}
	} else {
		log.Printf("LoaderDir not found: %s", loaderDir)
	}

	gioModPath := filepath.Join(bundlePath, "MacOS", "lib", "gio-modules")
	os.Setenv("GIO_MODULE_DIR", gioModPath)
	os.Setenv("GIO_EXTRA_MODULES", gioModPath)

	sharePath := filepath.Join(bundlePath, "Resources", "share")
	if _, err := os.Stat(sharePath); err == nil {
		os.Setenv("XDG_DATA_DIRS", sharePath)
	}
}

func buildHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: NETWORK_DIAL_TIMEOUT, KeepAlive: NETWORK_DIAL_KEEP_ALIVE}
				conn, err := dialer.DialContext(ctx, network, addr)
				if err == nil {
					return conn, nil
				}
				if !strings.Contains(err.Error(), "no such host") && !strings.Contains(err.Error(), "lookup") {
					return nil, err
				}
				log.Printf("DNS lookup failed for %s, retrying with 1.1.1.1...", addr)
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
						d := net.Dialer{Timeout: FALLBACK_DNS_DIAL_TIMEOUT}
						return d.DialContext(ctx, "udp", FALLBACK_DNS_RESOLVER_ENDPOINT)
					},
				}
				host, port, splitErr := net.SplitHostPort(addr)
				if splitErr != nil {
					return nil, err
				}
				ips, lookupErr := resolver.LookupIPAddr(ctx, host)
				if lookupErr != nil {
					log.Printf("fallback DNS lookup failed: %v", lookupErr)
					return nil, err
				}
				if len(ips) == 0 {
					return nil, err
				}
				targetAddr := net.JoinHostPort(ips[0].String(), port)
				return dialer.DialContext(ctx, network, targetAddr)
			},
			MaxIdleConns:          HTTP_MAX_IDLE_CONNS,
			MaxIdleConnsPerHost:   HTTP_MAX_IDLE_CONNS_PER_HOST,
			MaxConnsPerHost:       HTTP_MAX_CONNS_PER_HOST,
			IdleConnTimeout:       HTTP_IDLE_CONN_TIMEOUT,
			TLSHandshakeTimeout:   HTTP_TLS_HANDSHAKE_TIMEOUT,
			ResponseHeaderTimeout: HTTP_RESPONSE_HEADER_TIMEOUT,
			ExpectContinueTimeout: HTTP_EXPECT_CONTINUE_TIMEOUT,
		},
	}
}

func showMainWindow(app *gtk.Application, win *MainWindow) {
	win.SetApplicationForGTKWindow(app)
	win.BuildUI()
	app.AddWindow(win.window)
	if win.window != nil {
		win.window.ShowAll()
		win.PostShowInit()
	}
}

func showFatalDialogAndLog(prefix string, err error) {
	log.Printf("%s: %v", prefix, err)
	d := gtk.MessageDialogNew(nil, 0, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s: %v", prefix, err)
	d.Run()
	d.Destroy()
}

func bundledLoadersCachePath(bundlePath string) (string, bool) {
	cachePath := filepath.Join(bundlePath, "Resources", "loaders.cache")
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, true
	}
	return "", false
}

func bundledAdwaitaSymbolicIconPath(bundlePath, category, iconName string) (string, bool) {
	iconPath := filepath.Join(bundlePath, "Resources", "share", "icons", "Adwaita", "symbolic", category, iconName)
	if _, err := os.Stat(iconPath); err == nil {
		return iconPath, true
	}
	return "", false
}

func rewriteLoadersCachePaths(cacheOrig, runtimeLoaderDir string) (string, error) {
	data, err := os.ReadFile(cacheOrig)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, ".so\"") {
			soName := filepath.Base(strings.Trim(trimmed, "\""))
			lines[i] = "\"" + filepath.Join(runtimeLoaderDir, soName) + "\""
		}
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	cachePath := filepath.Join(cacheDir, "wiiu-loaders.cache")
	// Write to a temp file then atomically rename so a second concurrent
	// launch of the app can't leave readers with a torn cache.
	tmp, err := os.CreateTemp(cacheDir, "wiiu-loaders.cache.*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer func() {
		// If we never made it to the rename, clean up the dangling temp.
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write([]byte(strings.Join(lines, "\n"))); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		return "", err
	}
	// CreateTemp's default mode is 0600; restore 0644 to match prior behavior.
	if err := os.Chmod(cachePath, 0o644); err != nil {
		return "", err
	}
	return cachePath, nil
}

func regenerateLoadersCache(loaderDir string) {
	cacheDir, _ := os.UserCacheDir()
	cachePath := filepath.Join(cacheDir, "wiiu-loaders.cache")
	loaders, _ := filepath.Glob(filepath.Join(loaderDir, "*.so"))
	cacheData := buildLoadersCache(loaders)
	if err := os.WriteFile(cachePath, cacheData, 0o644); err == nil {
		os.Setenv("GDK_PIXBUF_MODULE_FILE", cachePath)
		log.Printf("Set GDK_PIXBUF_MODULE_FILE to regenerated %s", cachePath)
	} else {
		log.Printf("Failed to write regenerated cache: %v", err)
	}
}

func buildLoadersCache(loaders []string) []byte {
	var cache bytes.Buffer
	cache.WriteString("# GdkPixbuf Image Loader Modules\n# Automatically generated\n\n")
	for _, loader := range loaders {
		filename := filepath.Base(loader)
		entry := getLoaderEntry(filename, loader)
		if entry == "" {
			continue
		}
		cache.WriteString(entry)
		cache.WriteString("\n")
	}
	return cache.Bytes()
}

func getLoaderEntry(filename, path string) string {
	switch {
	case strings.Contains(filename, "svg"):
		return fmt.Sprintf("%q\n\"svg\" 6 \"gdk-pixbuf\" \"Scalable Vector Graphics\" \"LGPL\"\n\"image/svg+xml\" \"image/svg\" \"image/svg-xml\" \"image/vnd.adobe.svg+xml\" \"text/xml-svg\" \"image/svg+xml-compressed\" \"\"\n\"svg\" \"svgz\" \"svg.gz\" \"\"\n\" <svg\" \"*    \" 100\n\" <!DOCTYPE svg\" \"*             \" 100", path)
	case strings.Contains(filename, "bmp"):
		return fmt.Sprintf("%q\n\"bmp\" 5 \"gdk-pixbuf\" \"BMP\" \"LGPL\"\n\"image/bmp\" \"image/x-bmp\" \"image/x-MS-bmp\" \"\"\n\"bmp\" \"\"\n\"BM\" \"\" 100", path)
	case strings.Contains(filename, "gif"):
		return fmt.Sprintf("%q\n\"gif\" 4 \"gdk-pixbuf\" \"GIF\" \"LGPL\"\n\"image/gif\" \"\"\n\"gif\" \"\"\n\"GIF8\" \"\" 100", path)
	case strings.Contains(filename, "ico"):
		return fmt.Sprintf("%q\n\"ico\" 5 \"gdk-pixbuf\" \"Windows icon\" \"LGPL\"\n\"image/x-icon\" \"image/x-ico\" \"image/x-win-bitmap\" \"image/vnd.microsoft.icon\" \"application/ico\" \"image/ico\" \"image/icon\" \"text/ico\" \"\"\n\"ico\" \"cur\" \"\"\n\"  \\001   \" \"zz znz\" 100\n\"  \\002   \" \"zz znz\" 100", path)
	default:
		return ""
	}
}
