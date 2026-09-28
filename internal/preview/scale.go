package preview

import (
	"image"
	"image/color"

	"golang.org/x/image/draw"
)

func Scale(src image.Image, maxSide int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= maxSide && height <= maxSide {
		return src
	}

	scale := float64(maxSide) / float64(width)
	if height > width {
		scale = float64(maxSide) / float64(height)
	}
	dstWidth := max(1, int(float64(width)*scale))
	dstHeight := max(1, int(float64(height)*scale))

	// Average whole pixel blocks first so the remaining step is below 2x,
	// where bilinear sampling does not alias and is far cheaper than the
	// kernel scalers on multi-megapixel previews.
	if factor := min(width/dstWidth, height/dstHeight); factor >= 2 {
		src = shrinkBox(src, factor)
		bounds = src.Bounds()
		if bounds.Dx() == dstWidth && bounds.Dy() == dstHeight {
			return src
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Src, nil)
	return dst
}

// ToRGBA returns img as a zero-origin *image.RGBA, which Fyne uploads to the
// GPU without any per-frame conversion.
func ToRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok && rgba.Rect.Min == (image.Point{}) {
		return rgba
	}
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Rect, img, bounds.Min, draw.Src)
	return dst
}

func shrinkBox(src image.Image, factor int) *image.RGBA {
	if ycc, ok := src.(*image.YCbCr); ok {
		return shrinkYCbCr(ycc, factor)
	}
	return shrinkRGBA(ToRGBA(src), factor)
}

func shrinkYCbCr(src *image.YCbCr, factor int) *image.RGBA {
	bounds := src.Rect
	dstWidth := bounds.Dx() / factor
	dstHeight := bounds.Dy() / factor
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	count := uint32(factor * factor)
	ySums := make([]uint32, dstWidth)
	cbSums := make([]uint32, dstWidth)
	crSums := make([]uint32, dstWidth)

	for dy := 0; dy < dstHeight; dy++ {
		clear(ySums)
		clear(cbSums)
		clear(crSums)
		for sy := bounds.Min.Y + dy*factor; sy < bounds.Min.Y+(dy+1)*factor; sy++ {
			yRow := src.Y[src.YOffset(bounds.Min.X, sy):]
			for dx := 0; dx < dstWidth; dx++ {
				block := yRow[dx*factor : dx*factor+factor]
				var sum uint32
				for _, v := range block {
					sum += uint32(v)
				}
				ySums[dx] += sum
				for k := range factor {
					ci := src.COffset(bounds.Min.X+dx*factor+k, sy)
					cbSums[dx] += uint32(src.Cb[ci])
					crSums[dx] += uint32(src.Cr[ci])
				}
			}
		}

		row := dst.Pix[dy*dst.Stride:]
		for dx := 0; dx < dstWidth; dx++ {
			r, g, b := color.YCbCrToRGB(uint8(ySums[dx]/count), uint8(cbSums[dx]/count), uint8(crSums[dx]/count))
			pixel := row[dx*4 : dx*4+4]
			pixel[0], pixel[1], pixel[2], pixel[3] = r, g, b, 0xff
		}
	}
	return dst
}

func shrinkRGBA(src *image.RGBA, factor int) *image.RGBA {
	dstWidth := src.Rect.Dx() / factor
	dstHeight := src.Rect.Dy() / factor
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	count := uint32(factor * factor)
	sums := make([]uint32, dstWidth*4)

	for dy := 0; dy < dstHeight; dy++ {
		clear(sums)
		for sy := dy * factor; sy < (dy+1)*factor; sy++ {
			row := src.Pix[sy*src.Stride : sy*src.Stride+dstWidth*factor*4]
			for dx := 0; dx < dstWidth; dx++ {
				block := row[dx*factor*4 : (dx+1)*factor*4]
				sum := sums[dx*4 : dx*4+4]
				for k := 0; k < len(block); k += 4 {
					sum[0] += uint32(block[k])
					sum[1] += uint32(block[k+1])
					sum[2] += uint32(block[k+2])
					sum[3] += uint32(block[k+3])
				}
			}
		}
		row := dst.Pix[dy*dst.Stride : dy*dst.Stride+dstWidth*4]
		for i := range row {
			row[i] = uint8(sums[i] / count)
		}
	}
	return dst
}
