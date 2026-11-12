package gui

import (
	"bytes"
	"encoding/binary"
)

var (
	icoHeaderMagic = []byte{0x00, 0x00, 0x01, 0x00}
	pngHeaderMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
)

// icoToPNG extracts the largest embedded PNG frame from a Windows .ico
// container. If icon is not an .ico container or holds no PNG frames, it is
// returned unchanged.
//
// Wails on Windows and macOS accepts .ico containers for the tray icon, whereas
// its Linux StatusNotifierItem implementation passes the icon bytes straight to
// image/png.Decode.
func icoToPNG(icon []byte) []byte {
	const (
		headerSize = 6
		entrySize  = 16
	)
	if len(icon) < headerSize || !bytes.Equal(icon[:4], icoHeaderMagic) {
		return icon
	}

	count := int(binary.LittleEndian.Uint16(icon[4:6]))
	if count == 0 || len(icon) < headerSize+count*entrySize {
		return icon
	}

	var (
		best     []byte
		bestArea int
	)
	for i := 0; i < count; i++ {
		entry := icon[headerSize+i*entrySize : headerSize+(i+1)*entrySize]
		w := int(entry[0])
		if w == 0 {
			w = 256
		}
		h := int(entry[1])
		if h == 0 {
			h = 256
		}
		size := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if size < len(pngHeaderMagic) || offset < 0 || offset+size > len(icon) {
			continue
		}
		frame := icon[offset : offset+size]
		if !bytes.HasPrefix(frame, pngHeaderMagic) {
			continue
		}
		area := w * h
		if area > bestArea || (area == bestArea && len(frame) > len(best)) {
			best = frame
			bestArea = area
		}
	}
	if best == nil {
		return icon
	}
	return best
}
