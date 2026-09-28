package preview

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"image/jpeg"
	"io"
	"os"
)

const (
	tiffPreviewMaxIFDs    = 32
	tiffPreviewMaxEntries = 1024
	// A located preview much larger than needed is slow to decode; the full
	// scan may still find a smaller one (for example in maker notes).
	tiffPreviewMaxOversize = 4

	tagCompression       = 0x0103
	tagStripOffsets      = 0x0111
	tagOrientation       = 0x0112
	tagStripByteCounts   = 0x0117
	tagSubIFDs           = 0x014a
	tagJPEGOffset        = 0x0201
	tagJPEGLength        = 0x0202
	tagPanasonicJPEG     = 0x002e
	tiffTypeLong         = 4
	tiffTypeIFD          = 13
	compressionJPEG      = 6
	compressionJPEGNew   = 7
	compressionLossyJPEG = 34892
)

var errNoTIFFPreview = errors.New("no TIFF preview found")

type previewLocation struct {
	offset int64
	length int64
}

// readTIFFPreview follows TIFF IFD pointers (ARW, NEF, CR2, DNG, PEF, RW2 and
// similar) or the RAF header to the embedded JPEG previews, so only the
// directory entries and the chosen preview are read from disk. It returns the
// JPEG bytes and the IFD0 orientation.
func readTIFFPreview(file *os.File, maxSide int) ([]byte, int, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := info.Size()

	header := make([]byte, 92)
	n, _ := file.ReadAt(header, 0)
	header = header[:n]

	orientation := orientationNormal
	var locations []previewLocation
	if len(header) >= 92 && string(header[:16]) == "FUJIFILMCCD-RAW " {
		locations = append(locations, previewLocation{
			offset: int64(binary.BigEndian.Uint32(header[84:88])),
			length: int64(binary.BigEndian.Uint32(header[88:92])),
		})
	} else {
		order, ok := tiffByteOrder(header)
		if !ok {
			return nil, 0, errNoTIFFPreview
		}
		orientation, locations = walkTIFFPreviews(file, order, int64(order.Uint32(header[4:8])))
	}

	best, ok := choosePreviewLocation(file, size, locations, maxSide)
	if !ok {
		return nil, 0, errNoTIFFPreview
	}
	data := make([]byte, best.length)
	if _, err := file.ReadAt(data, best.offset); err != nil {
		return nil, 0, err
	}
	return data, orientation, nil
}

func tiffByteOrder(header []byte) (binary.ByteOrder, bool) {
	if len(header) < 8 {
		return nil, false
	}
	var order binary.ByteOrder
	switch string(header[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, false
	}
	switch order.Uint16(header[2:4]) {
	case 42, 0x55, 0x4f52, 0x5352: // TIFF, Panasonic RW2, Olympus ORF
		return order, true
	default:
		return nil, false
	}
}

func walkTIFFPreviews(file *os.File, order binary.ByteOrder, firstIFD int64) (int, []previewLocation) {
	orientation := orientationNormal
	var locations []previewLocation
	queue := []int64{firstIFD}
	visited := make(map[int64]bool)

	for len(queue) > 0 && len(visited) < tiffPreviewMaxIFDs {
		offset := queue[0]
		queue = queue[1:]
		if offset <= 0 || visited[offset] {
			continue
		}
		visited[offset] = true

		countBytes := make([]byte, 2)
		if _, err := file.ReadAt(countBytes, offset); err != nil {
			continue
		}
		count := int(order.Uint16(countBytes))
		if count == 0 || count > tiffPreviewMaxEntries {
			continue
		}
		entries := make([]byte, count*12+4)
		n, _ := file.ReadAt(entries, offset+2)
		if n < count*12 {
			continue
		}

		var compression, stripOffset, stripLength, jpegOffset, jpegLength int64
		for i := 0; i < count; i++ {
			entry := entries[i*12 : i*12+12]
			tag := order.Uint16(entry[0:2])
			fieldType := order.Uint16(entry[2:4])
			valueCount := order.Uint32(entry[4:8])
			value := tiffEntryValue(order, entry)
			switch tag {
			case tagOrientation:
				if len(visited) == 1 && value >= orientationNormal && value <= 8 {
					orientation = int(value)
				}
			case tagCompression:
				compression = value
			case tagStripOffsets:
				if valueCount == 1 {
					stripOffset = value
				}
			case tagStripByteCounts:
				if valueCount == 1 {
					stripLength = value
				}
			case tagJPEGOffset:
				jpegOffset = value
			case tagJPEGLength:
				jpegLength = value
			case tagPanasonicJPEG:
				locations = append(locations, previewLocation{offset: int64(order.Uint32(entry[8:12])), length: int64(valueCount)})
			case tagSubIFDs:
				if fieldType != tiffTypeLong && fieldType != tiffTypeIFD {
					continue
				}
				if valueCount == 1 {
					queue = append(queue, int64(order.Uint32(entry[8:12])))
					continue
				}
				pointers := make([]byte, min(int(valueCount), tiffPreviewMaxIFDs)*4)
				if _, err := file.ReadAt(pointers, int64(order.Uint32(entry[8:12]))); err != nil {
					continue
				}
				for p := 0; p+4 <= len(pointers); p += 4 {
					queue = append(queue, int64(order.Uint32(pointers[p:p+4])))
				}
			}
		}
		if jpegOffset > 0 && jpegLength > 0 {
			locations = append(locations, previewLocation{offset: jpegOffset, length: jpegLength})
		}
		if stripOffset > 0 && stripLength > 0 && (compression == compressionJPEG || compression == compressionJPEGNew || compression == compressionLossyJPEG) {
			locations = append(locations, previewLocation{offset: stripOffset, length: stripLength})
		}
		if n >= count*12+4 {
			queue = append(queue, int64(order.Uint32(entries[count*12:count*12+4])))
		}
	}
	return orientation, locations
}

func tiffEntryValue(order binary.ByteOrder, entry []byte) int64 {
	switch order.Uint16(entry[2:4]) {
	case 3: // SHORT
		return int64(order.Uint16(entry[8:10]))
	case tiffTypeLong, tiffTypeIFD:
		return int64(order.Uint32(entry[8:12]))
	default:
		return 0
	}
}

// choosePreviewLocation applies the same rule as embeddedJPEGChoice: the
// smallest preview that covers maxSide. It only accepts a result that the
// full scan could not improve on meaningfully.
func choosePreviewLocation(file *os.File, size int64, locations []previewLocation, maxSide int) (previewLocation, bool) {
	var best previewLocation
	bestArea := 0
	bestLongSide := 0
	for _, location := range locations {
		if location.offset <= 0 || location.length < 4 || location.length > embeddedJPEGMaxBytes || location.offset+location.length > size {
			continue
		}
		reader := bufio.NewReaderSize(io.NewSectionReader(file, location.offset, location.length), 64*1024)
		if soi, err := reader.Peek(3); err != nil || !bytes.Equal(soi, []byte{0xff, 0xd8, 0xff}) {
			continue
		}
		cfg, err := jpeg.DecodeConfig(reader)
		if err != nil {
			continue
		}
		longSide := max(cfg.Width, cfg.Height)
		area := cfg.Width * cfg.Height
		if longSide < maxSide {
			continue
		}
		if bestArea == 0 || area < bestArea {
			best = location
			bestArea = area
			bestLongSide = longSide
		}
	}
	if bestArea == 0 || bestLongSide > maxSide*tiffPreviewMaxOversize {
		return previewLocation{}, false
	}
	return best, true
}
