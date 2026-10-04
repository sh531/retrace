package exif

import "testing"

func TestTagString(t *testing.T) {
	for tg, want := range map[tag]string{tagMake: "Make", 0xABCD: "tag 0xABCD"} {
		if got := tg.String(); got != want {
			t.Errorf("tag(0x%04X).String() = %q, want %q", uint16(tg), got, want)
		}
	}
}
