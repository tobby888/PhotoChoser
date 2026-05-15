package preview

import (
	"encoding/binary"
	"image"
	"os"
)

const (
	orientationNormal  = 1
	orientationMaxRead = 1 << 20
)

func ReadOrientation(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return orientationNormal
	}
	defer file.Close()

	data := make([]byte, orientationMaxRead)
	n, err := file.Read(data)
	if err != nil && n == 0 {
		return orientationNormal
	}
	return readOrientationFromBytes(data[:n])
}

func ApplyOrientation(src image.Image, orientation int) image.Image {
	if orientation <= orientationNormal || orientation > 8 {
		return src
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width == 0 || height == 0 {
		return src
	}

	dstWidth := width
	dstHeight := height
	if orientation >= 5 {
		dstWidth = height
		dstHeight = width
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx, dy := orientedPoint(x, y, width, height, orientation)
			dst.Set(dx, dy, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}

func orientedPoint(x int, y int, width int, height int, orientation int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return height - 1 - y, x
	case 7:
		return height - 1 - y, width - 1 - x
	case 8:
		return y, width - 1 - x
	default:
		return x, y
	}
}

func readOrientationFromBytes(data []byte) int {
	if len(data) < 4 {
		return orientationNormal
	}
	if data[0] == 0xff && data[1] == 0xd8 {
		return readJPEGOrientation(data)
	}
	if isTIFFHeader(data) {
		return readTIFFOrientation(data, 0)
	}
	return orientationNormal
}

func readJPEGOrientation(data []byte) int {
	offset := 2
	for offset+4 <= len(data) {
		for offset < len(data) && data[offset] == 0xff {
			offset++
		}
		if offset >= len(data) {
			break
		}
		marker := data[offset]
		offset++
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if offset+2 > len(data) {
			break
		}
		segmentLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if segmentLength < 2 || offset+segmentLength > len(data) {
			break
		}
		segment := data[offset+2 : offset+segmentLength]
		if marker == 0xe1 && len(segment) >= 14 && string(segment[:6]) == "Exif\x00\x00" {
			if orientation := readTIFFOrientation(segment, 6); orientation != orientationNormal {
				return orientation
			}
		}
		offset += segmentLength
	}
	return orientationNormal
}

func readTIFFOrientation(data []byte, tiffStart int) int {
	if tiffStart < 0 || tiffStart+8 > len(data) || !isTIFFHeader(data[tiffStart:]) {
		return orientationNormal
	}

	var order binary.ByteOrder
	switch string(data[tiffStart : tiffStart+2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return orientationNormal
	}

	ifdOffset := int(order.Uint32(data[tiffStart+4 : tiffStart+8]))
	if ifdOffset < 0 || tiffStart+ifdOffset+2 > len(data) {
		return orientationNormal
	}
	ifdStart := tiffStart + ifdOffset
	entryCount := int(order.Uint16(data[ifdStart : ifdStart+2]))
	entriesStart := ifdStart + 2
	for i := 0; i < entryCount; i++ {
		entryStart := entriesStart + i*12
		if entryStart+12 > len(data) {
			return orientationNormal
		}
		tag := order.Uint16(data[entryStart : entryStart+2])
		if tag != 0x0112 {
			continue
		}
		fieldType := order.Uint16(data[entryStart+2 : entryStart+4])
		count := order.Uint32(data[entryStart+4 : entryStart+8])
		if fieldType != 3 || count < 1 {
			return orientationNormal
		}
		value := int(order.Uint16(data[entryStart+8 : entryStart+10]))
		if value >= orientationNormal && value <= 8 {
			return value
		}
		return orientationNormal
	}
	return orientationNormal
}

func isTIFFHeader(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return (string(data[:2]) == "II" && data[2] == 0x2a && data[3] == 0x00) ||
		(string(data[:2]) == "MM" && data[2] == 0x00 && data[3] == 0x2a)
}
