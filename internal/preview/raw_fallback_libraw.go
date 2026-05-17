//go:build cgo && libraw

package preview

/*
#include <stdlib.h>
#include <string.h>
#include <libraw/libraw.h>

static libraw_processed_image_t* photochoser_libraw_decode(const char* path, int* err) {
	libraw_data_t* raw = libraw_init(0);
	if (raw == NULL) {
		*err = LIBRAW_UNSPECIFIED_ERROR;
		return NULL;
	}

	*err = libraw_open_file(raw, path);
	if (*err != LIBRAW_SUCCESS) {
		libraw_close(raw);
		return NULL;
	}

	*err = libraw_unpack(raw);
	if (*err != LIBRAW_SUCCESS) {
		libraw_close(raw);
		return NULL;
	}

	raw->params.output_bps = 8;
	raw->params.use_camera_wb = 1;
	raw->params.no_auto_bright = 1;

	*err = libraw_dcraw_process(raw);
	if (*err != LIBRAW_SUCCESS) {
		libraw_close(raw);
		return NULL;
	}

	libraw_processed_image_t* image = libraw_dcraw_make_mem_image(raw, err);
	libraw_close(raw);
	return image;
}
*/
import "C"

import (
	"fmt"
	"image"
	"image/color"
	"unsafe"
)

func loadRAWFallback(path string) (image.Image, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var rawErr C.int
	decoded := C.photochoser_libraw_decode(cPath, &rawErr)
	if decoded == nil {
		return nil, fmt.Errorf("libraw decode failed: %s", C.GoString(C.libraw_strerror(rawErr)))
	}
	defer C.libraw_dcraw_clear_mem(decoded)

	if decoded._type != C.LIBRAW_IMAGE_BITMAP || decoded.bits != 8 || decoded.colors < 3 {
		return nil, fmt.Errorf("libraw returned unsupported image type=%d bits=%d colors=%d", decoded._type, decoded.bits, decoded.colors)
	}

	width := int(decoded.width)
	height := int(decoded.height)
	colors := int(decoded.colors)
	if width <= 0 || height <= 0 || colors <= 0 {
		return nil, fmt.Errorf("libraw returned invalid image dimensions %dx%d colors=%d", width, height, colors)
	}

	dataSize := int(decoded.data_size)
	needed := width * height * colors
	if dataSize < needed {
		return nil, fmt.Errorf("libraw returned short image buffer: got %d bytes, need %d", dataSize, needed)
	}

	src := unsafe.Slice((*byte)(unsafe.Pointer(&decoded.data[0])), dataSize)
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := (y*width + x) * colors
			dst.SetRGBA(x, y, color.RGBA{
				R: src[i],
				G: src[i+1],
				B: src[i+2],
				A: 255,
			})
		}
	}
	return dst, nil
}
