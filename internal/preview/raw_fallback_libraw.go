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
	for i, j := 0, 0; i < width*height*colors; i, j = i+colors, j+4 {
		dst.Pix[j] = src[i]
		dst.Pix[j+1] = src[i+1]
		dst.Pix[j+2] = src[i+2]
		dst.Pix[j+3] = 0xff
	}
	return dst, nil
}
