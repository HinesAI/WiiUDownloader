package main

import (
	"fmt"
	"image"
	"log"
	"sync/atomic"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
)

const (
	DETAIL_PANE_WIDTH       = 240
	DETAIL_COVER_SIZE       = 160
	DETAIL_LABEL_WRAP_CHARS = 28
)

type DetailPane struct {
	container     *gtk.Box
	coverImage    *gtk.Image
	nameLabel     *gtk.Label
	metaLabel     *gtk.Label
	sizeLabel     *gtk.Label
	yearLabel     *gtk.Label
	platformLabel *gtk.Label
	updateLabel   *gtk.Label
	dlcLabel      *gtk.Label
	emptyLabel    *gtk.Label
	contentBox    *gtk.Box

	requestID uint64
	sizeCache map[uint64]string
}

func NewDetailPane() (*DetailPane, error) {
	outer, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 8)
	if err != nil {
		return nil, err
	}
	outer.SetSizeRequest(DETAIL_PANE_WIDTH, -1)
	outer.SetMarginStart(8)
	outer.SetHAlign(gtk.ALIGN_FILL)
	addStyleClass(outer.GetStyleContext, "detail-pane")

	emptyLabel, err := gtk.LabelNew("Select a title to see cover art, size, and related content.")
	if err != nil {
		return nil, err
	}
	emptyLabel.SetLineWrap(true)
	emptyLabel.SetMaxWidthChars(DETAIL_LABEL_WRAP_CHARS)
	emptyLabel.SetJustify(gtk.JUSTIFY_CENTER)
	emptyLabel.SetHAlign(gtk.ALIGN_CENTER)
	emptyLabel.SetVAlign(gtk.ALIGN_CENTER)
	emptyLabel.SetVExpand(true)
	emptyLabel.SetMarginTop(24)
	emptyLabel.SetMarginBottom(24)

	contentBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 10)
	if err != nil {
		return nil, err
	}
	contentBox.SetHAlign(gtk.ALIGN_FILL)

	coverImage, err := gtk.ImageNew()
	if err != nil {
		return nil, err
	}
	coverImage.SetSizeRequest(DETAIL_COVER_SIZE, DETAIL_COVER_SIZE)
	coverImage.SetHAlign(gtk.ALIGN_CENTER)
	coverImage.SetPixelSize(DETAIL_COVER_SIZE)

	nameLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	nameLabel.SetLineWrap(true)
	nameLabel.SetMaxWidthChars(DETAIL_LABEL_WRAP_CHARS)
	nameLabel.SetJustify(gtk.JUSTIFY_CENTER)
	nameLabel.SetHAlign(gtk.ALIGN_CENTER)
	nameLabel.SetEllipsize(pango.ELLIPSIZE_END)

	metaLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	metaLabel.SetLineWrap(true)
	metaLabel.SetMaxWidthChars(DETAIL_LABEL_WRAP_CHARS)
	metaLabel.SetJustify(gtk.JUSTIFY_CENTER)
	metaLabel.SetHAlign(gtk.ALIGN_CENTER)
	metaLabel.SetSelectable(true)

	sizeLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	sizeLabel.SetHAlign(gtk.ALIGN_START)
	sizeLabel.SetLineWrap(true)

	yearLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	yearLabel.SetHAlign(gtk.ALIGN_START)
	yearLabel.SetLineWrap(true)

	platformLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	platformLabel.SetHAlign(gtk.ALIGN_START)
	platformLabel.SetLineWrap(true)

	updateLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	updateLabel.SetHAlign(gtk.ALIGN_START)
	updateLabel.SetLineWrap(true)
	updateLabel.SetMaxWidthChars(DETAIL_LABEL_WRAP_CHARS)

	dlcLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	dlcLabel.SetHAlign(gtk.ALIGN_START)
	dlcLabel.SetLineWrap(true)
	dlcLabel.SetMaxWidthChars(DETAIL_LABEL_WRAP_CHARS)

	contentBox.PackStart(coverImage, false, false, 0)
	contentBox.PackStart(nameLabel, false, false, 0)
	contentBox.PackStart(metaLabel, false, false, 0)
	contentBox.PackStart(sizeLabel, false, false, 0)
	contentBox.PackStart(yearLabel, false, false, 0)
	contentBox.PackStart(platformLabel, false, false, 0)
	contentBox.PackStart(updateLabel, false, false, 0)
	contentBox.PackStart(dlcLabel, false, false, 0)

	outer.PackStart(emptyLabel, true, true, 0)
	outer.PackStart(contentBox, true, true, 0)
	contentBox.Hide()

	return &DetailPane{
		container:     outer,
		coverImage:    coverImage,
		nameLabel:     nameLabel,
		metaLabel:     metaLabel,
		sizeLabel:     sizeLabel,
		yearLabel:     yearLabel,
		platformLabel: platformLabel,
		updateLabel:   updateLabel,
		dlcLabel:      dlcLabel,
		emptyLabel:    emptyLabel,
		contentBox:    contentBox,
		sizeCache:     make(map[uint64]string),
	}, nil
}

func (dp *DetailPane) GetContainer() *gtk.Box {
	return dp.container
}

func (dp *DetailPane) Clear() {
	atomic.AddUint64(&dp.requestID, 1)
	dp.emptyLabel.Show()
	dp.contentBox.Hide()
}

func (dp *DetailPane) ShowEntry(entry wiiudownloader.TitleEntry, fetchSize func(uint64) (uint64, error)) {
	if entry.TitleID == 0 {
		dp.Clear()
		return
	}

	reqID := atomic.AddUint64(&dp.requestID, 1)

	dp.emptyLabel.Hide()
	dp.contentBox.Show()
	dp.coverImage.Show()
	dp.nameLabel.Show()
	dp.metaLabel.Show()
	dp.sizeLabel.Show()
	dp.yearLabel.Show()
	dp.platformLabel.Show()

	dp.nameLabel.SetMarkup(fmt.Sprintf("<span weight='bold' size='large'>%s</span>", escapeMarkup(entry.Name)))
	dp.metaLabel.SetText(fmt.Sprintf("%s · %s\n%016X",
		wiiudownloader.GetFormattedKind(entry.TitleID),
		wiiudownloader.GetFormattedRegion(entry.Region),
		entry.TitleID,
	))

	dp.coverImage.Clear()
	dp.coverImage.SetFromIconName("image-x-generic", gtk.ICON_SIZE_DIALOG)
	dp.yearLabel.SetMarkup("<b>Year:</b> Loading…")
	dp.platformLabel.SetMarkup("<b>System:</b> Loading…")
	dp.setRelatedLabels(entry)

	if cached, ok := dp.sizeCache[entry.TitleID]; ok {
		dp.sizeLabel.SetMarkup(fmt.Sprintf("<b>Size:</b> %s", escapeMarkup(cached)))
	} else {
		dp.sizeLabel.SetMarkup("<b>Size:</b> Loading…")
		go dp.loadSize(reqID, entry.TitleID, fetchSize)
	}

	go dp.loadCover(reqID, entry.TitleID)
	go dp.loadGameTDBMeta(reqID, entry)
}

func (dp *DetailPane) setRelatedLabels(entry wiiudownloader.TitleEntry) {
	high := wiiudownloader.GetTitleIDHigh(entry.TitleID)

	switch high {
	case wiiudownloader.TID_HIGH_GAME:
		update, hasUpdate, dlc, hasDLC := wiiudownloader.FindRelatedUpdateAndDLC(entry)
		dp.updateLabel.SetMarkup(formatAvailability("Update", hasUpdate, update))
		dp.dlcLabel.SetMarkup(formatAvailability("DLC", hasDLC, dlc))
		dp.updateLabel.Show()
		dp.dlcLabel.Show()
	case wiiudownloader.TID_HIGH_UPDATE:
		game, hasGame := wiiudownloader.FindRelatedTitleByHighAndLow(entry, wiiudownloader.TID_HIGH_GAME, map[uint64]struct{}{entry.TitleID: {}})
		_, _, dlc, hasDLC := wiiudownloader.FindRelatedUpdateAndDLC(entry)
		dp.updateLabel.SetMarkup(formatAvailability("Base game", hasGame, game))
		dp.dlcLabel.SetMarkup(formatAvailability("DLC", hasDLC, dlc))
		dp.updateLabel.Show()
		dp.dlcLabel.Show()
	case wiiudownloader.TID_HIGH_DLC:
		game, hasGame := wiiudownloader.FindRelatedTitleByHighAndLow(entry, wiiudownloader.TID_HIGH_GAME, map[uint64]struct{}{entry.TitleID: {}})
		update, hasUpdate, _, _ := wiiudownloader.FindRelatedUpdateAndDLC(entry)
		dp.updateLabel.SetMarkup(formatAvailability("Base game", hasGame, game))
		dp.dlcLabel.SetMarkup(formatAvailability("Update", hasUpdate, update))
		dp.updateLabel.Show()
		dp.dlcLabel.Show()
	default:
		dp.updateLabel.SetText("")
		dp.dlcLabel.SetText("")
		dp.updateLabel.Hide()
		dp.dlcLabel.Hide()
	}
}

func formatAvailability(kind string, available bool, entry wiiudownloader.TitleEntry) string {
	if !available {
		return fmt.Sprintf("<b>%s:</b> Not available", escapeMarkup(kind))
	}
	return fmt.Sprintf("<b>%s:</b> Available\n%s", escapeMarkup(kind), escapeMarkup(entry.Name))
}

func (dp *DetailPane) loadSize(reqID, titleID uint64, fetchSize func(uint64) (uint64, error)) {
	if fetchSize == nil {
		uiIdleAdd(func() {
			if atomic.LoadUint64(&dp.requestID) != reqID {
				return
			}
			dp.sizeLabel.SetMarkup("<b>Size:</b> Unavailable")
		})
		return
	}

	size, err := fetchSize(titleID)
	uiIdleAdd(func() {
		if atomic.LoadUint64(&dp.requestID) != reqID {
			return
		}
		if err != nil {
			log.Printf("detail size fetch failed for %016x: %v", titleID, err)
			dp.sizeLabel.SetMarkup("<b>Size:</b> Unavailable")
			return
		}
		formatted := formatBytes(size)
		dp.sizeCache[titleID] = formatted
		dp.sizeLabel.SetMarkup(fmt.Sprintf("<b>Size:</b> %s", escapeMarkup(formatted)))
	})
}

func (dp *DetailPane) loadGameTDBMeta(reqID uint64, entry wiiudownloader.TitleEntry) {
	meta, ok := lookupGameTDBMeta(entry)
	uiIdleAdd(func() {
		if atomic.LoadUint64(&dp.requestID) != reqID {
			return
		}
		if !ok {
			if _, err := ensureGameTDBIndex(); err != nil {
				dp.yearLabel.SetMarkup("<b>Year:</b> Unavailable")
				dp.platformLabel.SetMarkup("<b>System:</b> Unavailable")
				return
			}
			dp.yearLabel.SetMarkup("<b>Year:</b> Unknown")
			dp.platformLabel.SetMarkup("<b>System:</b> Unknown")
			return
		}
		if meta.Year != "" {
			dp.yearLabel.SetMarkup(fmt.Sprintf("<b>Year:</b> %s", escapeMarkup(meta.Year)))
		} else {
			dp.yearLabel.SetMarkup("<b>Year:</b> Unknown")
		}
		if meta.Platform != "" {
			dp.platformLabel.SetMarkup(fmt.Sprintf("<b>System:</b> %s", escapeMarkup(meta.Platform)))
		} else {
			dp.platformLabel.SetMarkup("<b>System:</b> Unknown")
		}
	})
}

func (dp *DetailPane) loadCover(reqID, titleID uint64) {
	img, err := fetchTitleCoverImage(titleID)
	uiIdleAdd(func() {
		if atomic.LoadUint64(&dp.requestID) != reqID {
			return
		}
		if err != nil {
			log.Printf("detail cover fetch failed for %016x: %v", titleID, err)
			dp.coverImage.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
			return
		}
		pixbuf, pixbufErr := pixbufFromImage(img)
		if pixbufErr != nil || pixbuf == nil {
			log.Printf("detail cover pixbuf failed for %016x: %v", titleID, pixbufErr)
			dp.coverImage.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
			return
		}
		scaled, scaleErr := pixbuf.ScaleSimple(DETAIL_COVER_SIZE, DETAIL_COVER_SIZE, gdk.INTERP_BILINEAR)
		if scaleErr != nil || scaled == nil {
			dp.coverImage.SetFromPixbuf(pixbuf)
			return
		}
		dp.coverImage.SetFromPixbuf(scaled)
	})
}

// pixbufFromImage builds a GdkPixbuf from raw pixels so we do not depend on
// gdk-pixbuf PNG loaders (often missing from the macOS app bundle module cache).
func pixbufFromImage(img image.Image) (*gdk.Pixbuf, error) {
	nrgba := imageToNRGBA(img)
	bounds := nrgba.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid image size")
	}
	pix := make([]byte, len(nrgba.Pix))
	copy(pix, nrgba.Pix)
	return gdk.PixbufNewFromData(pix, gdk.COLORSPACE_RGB, true, 8, width, height, nrgba.Stride)
}
