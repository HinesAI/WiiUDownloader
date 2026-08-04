package main

import (
	"fmt"
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
	container   *gtk.Box
	coverImage  *gtk.Image
	nameLabel   *gtk.Label
	metaLabel   *gtk.Label
	sizeLabel   *gtk.Label
	updateLabel *gtk.Label
	dlcLabel    *gtk.Label
	emptyLabel  *gtk.Label
	contentBox  *gtk.Box

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
	contentBox.SetNoShowAll(true)

	coverImage, err := gtk.ImageNew()
	if err != nil {
		return nil, err
	}
	coverImage.SetSizeRequest(DETAIL_COVER_SIZE, DETAIL_COVER_SIZE)
	coverImage.SetHAlign(gtk.ALIGN_CENTER)
	coverImage.SetFromIconName("image-x-generic", gtk.ICON_SIZE_DIALOG)

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
	contentBox.PackStart(updateLabel, false, false, 0)
	contentBox.PackStart(dlcLabel, false, false, 0)

	outer.PackStart(emptyLabel, true, true, 0)
	outer.PackStart(contentBox, true, true, 0)

	return &DetailPane{
		container:   outer,
		coverImage:  coverImage,
		nameLabel:   nameLabel,
		metaLabel:   metaLabel,
		sizeLabel:   sizeLabel,
		updateLabel: updateLabel,
		dlcLabel:    dlcLabel,
		emptyLabel:  emptyLabel,
		contentBox:  contentBox,
		sizeCache:   make(map[uint64]string),
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
	dp.contentBox.ShowAll()

	dp.nameLabel.SetMarkup(fmt.Sprintf("<span weight='bold'>%s</span>", escapeMarkup(entry.Name)))
	dp.metaLabel.SetText(fmt.Sprintf("%s · %s\n%016X",
		wiiudownloader.GetFormattedKind(entry.TitleID),
		wiiudownloader.GetFormattedRegion(entry.Region),
		entry.TitleID,
	))

	dp.coverImage.SetFromIconName("image-x-generic", gtk.ICON_SIZE_DIALOG)
	dp.setRelatedLabels(entry)

	if cached, ok := dp.sizeCache[entry.TitleID]; ok {
		dp.sizeLabel.SetMarkup(fmt.Sprintf("<b>Size:</b> %s", escapeMarkup(cached)))
	} else {
		dp.sizeLabel.SetMarkup("<b>Size:</b> Loading…")
		go dp.loadSize(reqID, entry.TitleID, fetchSize)
	}

	go dp.loadCover(reqID, entry.TitleID)
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

func (dp *DetailPane) loadCover(reqID, titleID uint64) {
	pngData, err := fetchTitleCoverPNG(titleID)
	uiIdleAdd(func() {
		if atomic.LoadUint64(&dp.requestID) != reqID {
			return
		}
		if err != nil {
			log.Printf("detail cover fetch failed for %016x: %v", titleID, err)
			dp.coverImage.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
			return
		}
		loader, loaderErr := gdk.PixbufLoaderNew()
		if loaderErr != nil {
			dp.coverImage.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
			return
		}
		if _, writeErr := loader.Write(pngData); writeErr != nil {
			_ = loader.Close()
			dp.coverImage.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
			return
		}
		if closeErr := loader.Close(); closeErr != nil {
			dp.coverImage.SetFromIconName("image-missing", gtk.ICON_SIZE_DIALOG)
			return
		}
		pixbuf, pixbufErr := loader.GetPixbuf()
		if pixbufErr != nil || pixbuf == nil {
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
