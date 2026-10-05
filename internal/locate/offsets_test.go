package locate

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sh531/retrace/internal/exif"
)

// *Offsets must satisfy flag.Value; this fails to compile if it doesn't.
var _ flag.Value = (*Offsets)(nil)

// Reference photos from internal/exif/testdata.
var (
	sonyJPEG    = filepath.Join("..", "exif", "testdata", "sony.jpg")           // SONY ILCE-9
	appleJPEG   = filepath.Join("..", "exif", "testdata", "apple.jpg")          // Apple iPhone 13 Pro
	makeOnlyJPG = filepath.Join("..", "exif", "testdata", "gps_south_east.jpg") // Apple, no Model
	noEXIFJPEG  = filepath.Join("..", "exif", "testdata", "no_exif.jpg")
)

func TestOffsetsSet(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    string // String() after setting values
		wantErr string // substring; empty means success
	}{
		{name: "none", want: ""},
		{name: "one", values: []string{sonyJPEG + "=2m30s"}, want: sonyJPEG + "=2m30s"},
		{name: "negative", values: []string{sonyJPEG + "=-1m15s"}, want: sonyJPEG + "=-1m15s"},
		{name: "two cameras, sorted by photo", values: []string{sonyJPEG + "=1s", appleJPEG + "= 0s"}, want: appleJPEG + "=0s," + sonyJPEG + "=1s"},
		{name: "make without model", values: []string{makeOnlyJPG + "=1s"}, want: makeOnlyJPG + "=1s"},
		{name: "no =", values: []string{sonyJPEG}, wantErr: "want photo=duration"},
		{name: "no photo", values: []string{"=2m"}, wantErr: "missing photo before ="},
		{name: "bad duration", values: []string{sonyJPEG + "=2 minutes"}, wantErr: `unknown unit " minutes"`},
		{name: "no unit", values: []string{sonyJPEG + "=90"}, wantErr: "missing unit"},
		{name: "missing photo", values: []string{"missing.jpg=1m"}, wantErr: "missing.jpg: no such file"},
		{name: "not a JPEG", values: []string{filepath.Join("..", "exif", "testdata", "gen.sh") + "=1m"}, wantErr: "not a JPEG"},
		{name: "no camera in EXIF", values: []string{noEXIFJPEG + "=1m"}, wantErr: "has neither a camera make nor a model"},
		{name: "same photo twice", values: []string{sonyJPEG + "=1s", sonyJPEG + "=2s"}, wantErr: sonyJPEG + " given twice"},
		{name: "empty", values: []string{""}, wantErr: "want photo=duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o Offsets
			var err error
			for _, v := range tt.values {
				if err = o.Set(v); err != nil {
					break
				}
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Set() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Set() unexpected error: %v", err)
			}
			if got := o.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOffsetsSetPathWithEquals(t *testing.T) {
	data, err := os.ReadFile(sonyJPEG)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a=b.jpg")
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // G703: path is t.TempDir() plus a fixed name
		t.Fatal(err)
	}

	var o Offsets
	if err := o.Set(path + "=1m"); err != nil {
		t.Fatalf("Set() unexpected error: %v", err)
	}
	if got := o.For(sony); got != time.Minute {
		t.Errorf("For(%+v) = %v, want 1m", sony, got)
	}
}

func TestOffsetsFor(t *testing.T) {
	var o Offsets
	if err := o.Set(sonyJPEG + "=2m"); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		c    exif.Camera
		want time.Duration
	}{
		{"same camera", sony, 2 * time.Minute},
		{"same make, other model", exif.Camera{Make: "SONY", Model: "ILCE-7RM5"}, 0},
		{"other camera", apple, 0},
		{"no EXIF", exif.Camera{}, 0},
	} {
		if got := o.For(tt.c); got != tt.want {
			t.Errorf("%s: For(%+v) = %v, want %v", tt.name, tt.c, got, tt.want)
		}
	}

	var zero Offsets
	if got := zero.For(sony); got != 0 {
		t.Errorf("zero Offsets: For(%+v) = %v, want 0", sony, got)
	}
}

func TestOffsetsSameCamera(t *testing.T) {
	data, err := os.ReadFile(sonyJPEG)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other.jpg")
	if err := os.WriteFile(other, data, 0o600); err != nil { //nolint:gosec // G703: path is t.TempDir() plus a fixed name
		t.Fatal(err)
	}

	var o Offsets
	if err := o.Set(sonyJPEG + "=1s"); err != nil {
		t.Fatal(err)
	}
	err = o.Set(other + "=2s")
	want := other + " is from the same camera (SONY ILCE-9) as " + sonyJPEG
	if err == nil || err.Error() != want {
		t.Errorf("Set() error = %v, want %q", err, want)
	}
}

func TestOffsetsString(t *testing.T) {
	var nilOffsets *Offsets
	if got := nilOffsets.String(); got != "" {
		t.Errorf("nil *Offsets: String() = %q, want empty", got)
	}
	if got := (&Offsets{}).String(); got != "" {
		t.Errorf("zero Offsets: String() = %q, want empty", got)
	}
}

func TestOffsetsFlag(t *testing.T) {
	// newFlagSet returns a FlagSet with -offset, writing usage and errors to out.
	newFlagSet := func(o *Offsets, out io.Writer) *flag.FlagSet {
		fs := flag.NewFlagSet("retrace", flag.ContinueOnError)
		fs.SetOutput(out)
		fs.Var(o, "offset", "correct the clock of the camera that took a `photo=duration`")
		return fs
	}

	t.Run("repeated", func(t *testing.T) {
		var o Offsets
		fs := newFlagSet(&o, io.Discard)
		if err := fs.Parse([]string{"--offset", sonyJPEG + "=3m", "--offset=" + appleJPEG + "=-1s"}); err != nil {
			t.Fatalf("Parse() unexpected error: %v", err)
		}
		if got, want := o.String(), appleJPEG+"=-1s,"+sonyJPEG+"=3m0s"; got != want {
			t.Errorf("after Parse(), String() = %q, want %q", got, want)
		}
	})

	for _, tt := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"invalid value", []string{"--offset", "sony"}, `invalid value "sony" for flag -offset: want photo=duration`},
		{"empty value", []string{"--offset="}, `invalid value "" for flag -offset: want photo=duration`},
		{"missing value", []string{"--offset"}, "flag needs an argument: -offset"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var o Offsets
			err := newFlagSet(&o, io.Discard).Parse(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse(%q) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
			}
		})
	}

	t.Run("usage", func(t *testing.T) {
		var o Offsets
		var out strings.Builder
		newFlagSet(&o, &out).PrintDefaults()
		got := out.String()
		if !strings.Contains(got, "-offset photo=duration") {
			t.Errorf("PrintDefaults() = %q, want the argument named photo=duration", got)
		}
		// flag prints "(default …)" if String differs for a zero value, and a
		// "panic calling String method" note if String panics.
		if strings.Contains(got, "default") || strings.Contains(got, "panic") {
			t.Errorf("PrintDefaults() = %q, want no default or panic note", got)
		}
	})
}
