package main

import (
	"bytes"
	"github.com/dvyukov/go-fuzz/gen"
	"image"
	"image/color"
	"image/png"
)

func main() {
	for {
		img := createRandomImage(256)
		buf := new(bytes.Buffer)
		enc := &png.Encoder{CompressionLevel: png.CompressionLevel(gen.Rand(4) - 3)}
		if err := enc.Encode(buf, img); err != nil {
			panic(err)
		}
		gen.Emit(buf.Bytes(), nil, true)
	}
}

type Image interface {
	Set(x, y int, c color.Color)
}

func createRandomImage(maxPaletteSize int) image.Image {
	r := image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{gen.Rand(32) + 1, gen.Rand(32) + 1}}
	var img Image
	switch gen.Rand(10) {
	case 0:
		img = image.NewAlpha(r)
	case 1:
		img = image.NewAlpha16(r)
	case 2:
		img = image.NewCMYK(r)
	case 3:
		img = image.NewGray(r)
	case 4:
		img = image.NewGray16(r)
	case 5:
		img = image.NewNRGBA(r)
	case 6:
		img = image.NewNRGBA64(r)
	case 7:
		img = image.NewPaletted(r, randPalette(maxPaletteSize))
	case 8:
		img = image.NewRGBA(r)
	case 9:
		img = image.NewRGBA64(r)
	default:
		panic("bad")
	}
	fill := gen.Rand(19)
	var palette []color.Color
	if fill == 17 {
		palette = randPalette(maxPaletteSize)
	}
	for y := 0; y < r.Max.Y; y++ {
		for x := 0; x < r.Max.X; x++ {
			switch {
			case fill <= 15:
				img.Set(x, y, color.RGBA64{
					^uint16(0) * uint16((fill>>0)&1),
					^uint16(0) * uint16((fill>>1)&1),
					^uint16(0) * uint16((fill>>2)&1),
					^uint16(0) * uint16((fill>>3)&1),
				})
			case fill == 16:
				img.Set(x, y, randColor())
			case fill == 17:
				img.Set(x, y, palette[gen.Rand(len(palette))])
			case fill == 18:
				if gen.Rand(3) != 0 {
					img.Set(x, y, color.RGBA64{})
				} else {
					img.Set(x, y, randColor())
				}
			default:
				panic("bad")
			}
		}
	}
	return img.(image.Image)
}

func randColor() color.Color {
	return color.RGBA64{
		uint16(gen.Rand(1 << 16)),
		uint16(gen.Rand(1 << 16)),
		uint16(gen.Rand(1 << 16)),
		uint16(gen.Rand(1 << 16)),
	}
}

func randPalette(maxPaletteSize int) color.Palette {
	palette := make([]color.Color, gen.Rand(maxPaletteSize)+1)
	for i := range palette {
		palette[i] = randColor()
	}
	return palette
}
