package exif

import (
	"errors"
	"testing"
)

func TestSetErrKeepsFirst(t *testing.T) {
	var d decoder
	first := errors.New("first")
	d.setErr(first)
	d.setErr(errors.New("second"))
	d.tagErrorf(tagMake, "third")
	if !errors.Is(d.err, first) {
		t.Errorf("err = %v, want the first error", d.err)
	}
}
