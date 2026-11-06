package clipboard

import (
	"encoding/binary"
	"testing"
)

func TestDibDimensions(t *testing.T) {
	makeDib := func(width, height int32) []byte {
		dib := make([]byte, 40)
		binary.LittleEndian.PutUint32(dib[0:], 40)
		binary.LittleEndian.PutUint32(dib[4:], uint32(width))
		binary.LittleEndian.PutUint32(dib[8:], uint32(height))
		return dib
	}

	cases := []struct {
		name       string
		dib        []byte
		wantWidth  int
		wantHeight int
		wantOK     bool
	}{
		{"bottom-up", makeDib(640, 480), 640, 480, true},
		{"top-down", makeDib(640, -480), 640, 480, true},
		{"zero width", makeDib(0, 480), 0, 0, false},
		{"zero height", makeDib(640, 0), 0, 0, false},
		{"negative width", makeDib(-640, 480), 0, 0, false},
		{"truncated header", makeDib(640, 480)[:11], 0, 0, false},
		{"empty", nil, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			width, height, ok := DibDimensions(tc.dib)
			if ok != tc.wantOK || width != tc.wantWidth || height != tc.wantHeight {
				t.Fatalf("DibDimensions() = (%d, %d, %v), want (%d, %d, %v)",
					width, height, ok, tc.wantWidth, tc.wantHeight, tc.wantOK)
			}
		})
	}
}
