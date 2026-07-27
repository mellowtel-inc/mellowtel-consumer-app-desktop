package config

import "testing"

func TestSharingIntensityWorkers(t *testing.T) {
	// The UI never shows these numbers, but the ordering must hold: more
	// intensity means more concurrent jobs.
	low := IntensityLow.Workers()
	med := IntensityMedium.Workers()
	max := IntensityMax.Workers()

	if !(low < med && med < max) {
		t.Errorf("expected low < medium < max, got %d, %d, %d", low, med, max)
	}
	if low < 1 {
		t.Errorf("low intensity must still run at least one job, got %d", low)
	}
}

func TestSharingIntensityUnknownFallsBackToMedium(t *testing.T) {
	var bogus SharingIntensity = "banana"
	if bogus.Workers() != IntensityMedium.Workers() {
		t.Errorf("unknown intensity should behave like medium")
	}
}

func TestNormalizeRepairsInvalidIntensity(t *testing.T) {
	c := Defaults()
	c.Settings.SharingIntensity = "nonsense"
	c.normalize()
	if c.Settings.SharingIntensity != IntensityMedium {
		t.Errorf("normalize() = %q, want medium", c.Settings.SharingIntensity)
	}
}

func TestIntensityDescriptionsPresent(t *testing.T) {
	for _, i := range []SharingIntensity{IntensityLow, IntensityMedium, IntensityMax} {
		if i.Description() == "" {
			t.Errorf("intensity %q has no description", i)
		}
	}
}

func TestBandwidthCapBytes(t *testing.T) {
	if CapUnlimited.BytesPerDay() != 0 {
		t.Errorf("unlimited should be 0 (no cap)")
	}
	if Cap1GB.BytesPerDay() >= Cap5GB.BytesPerDay() {
		t.Errorf("1GB cap should be smaller than 5GB cap")
	}
}
