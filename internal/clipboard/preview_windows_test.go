package clipboard

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDIBPreviewSamplesPixelsAndHandlesOrientation(t *testing.T) {
	for _, topDown := range []bool{false, true} {
		dib := make([]byte, 40+16)
		binary.LittleEndian.PutUint32(dib[0:4], 40)
		binary.LittleEndian.PutUint32(dib[4:8], 2)
		height := int32(2)
		if topDown {
			height = -height
		}
		binary.LittleEndian.PutUint32(dib[8:12], uint32(height))
		binary.LittleEndian.PutUint16(dib[12:14], 1)
		binary.LittleEndian.PutUint16(dib[14:16], 32)
		red := []byte{0, 0, 255, 255, 0, 0, 255, 255}
		blue := []byte{255, 0, 0, 255, 255, 0, 0, 255}
		if topDown {
			copy(dib[40:], red)
			copy(dib[48:], blue)
		} else {
			copy(dib[40:], blue)
			copy(dib[48:], red)
		}
		preview, err := DIBPreview(bytes.NewReader(dib), int64(len(dib)), 1)
		if err != nil || preview.Bounds().Dx() != 1 || preview.RGBAAt(0, 0).R != 255 || preview.RGBAAt(0, 0).B != 0 {
			t.Fatalf("topDown=%v preview=%v err=%v", topDown, preview, err)
		}
	}
}
