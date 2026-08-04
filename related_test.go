package wiiudownloader

import "testing"

func TestFindRelatedUpdateAndDLC(t *testing.T) {
	if len(TitleDatabase) == 0 {
		t.Skip("title database not loaded")
	}
	entry := GetTitleEntryFromTid(0x000500001010EC00) // Mario Kart 8 USA
	if entry.TitleID == 0 {
		t.Skip("Mario Kart 8 USA not in database")
	}
	update, hasUpdate, dlc, hasDLC := FindRelatedUpdateAndDLC(entry)
	if !hasUpdate {
		t.Fatalf("expected update for Mario Kart 8")
	}
	if GetTitleIDHigh(update.TitleID) != TID_HIGH_UPDATE {
		t.Fatalf("update high bits wrong: %016x", update.TitleID)
	}
	if !hasDLC {
		t.Fatalf("expected DLC for Mario Kart 8")
	}
	if GetTitleIDHigh(dlc.TitleID) != TID_HIGH_DLC {
		t.Fatalf("dlc high bits wrong: %016x", dlc.TitleID)
	}
}
