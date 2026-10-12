package main

import (
	"math"
	"strings"
	"testing"
)

func TestPluginPageBoundsRefuseInvalidGeometryBeforeNativeAdmission(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		if err := (PluginPageBounds{Width: value}).validate(); err == nil {
			t.Fatalf("accepted %v", value)
		}
	}
	host := mustDesktopHost(t, t.TempDir())
	if _, err := host.OpenPluginPage(PluginPageBounds{Width: 100, Height: 100}); err == nil {
		t.Fatal("opened a page without a native window")
	}
	if err := host.SendPluginPage("unknown", "not JSON"); err == nil {
		t.Fatal("admitted invalid bridge JSON")
	}
}

func TestNativePublicationRetainsItsPhysicalFrameLimit(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	frame := `{"type":"fragment","index":0,"last":true,"data":"` + strings.Repeat("x", maxPluginMessageBytes) + `"}`
	if err := host.SendPluginPage("unallocated", frame); err == nil {
		t.Fatal("oversized native frame was admitted")
	}
}
