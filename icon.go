package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/draw"
)

// The icon of the agent, 512 dots. It is the one next to the clock on Windows; the app of the Mac
// and the program of Windows get theirs from the same file, in build.sh.
//
//go:embed icon.png
var iconFile []byte

// The menu bar of the Mac wants a drawing of one colour, which macOS paints black or white: it is
// drawn here, a sheet crossed by the line of the scanner, 32 dots wide.
const iconSize = 32

// glyph says how much of a dot belongs to the drawing: the outline of the sheet and the line.
func glyph(x, y int) bool {
	sheet := x >= 9 && x <= 22 && y >= 3 && y <= 28
	inside := x >= 11 && x <= 20 && y >= 5 && y <= 26
	line := x >= 4 && x <= 27 && y >= 15 && y <= 16
	gap := x >= 7 && x <= 24 && y >= 13 && y <= 18
	return line || (sheet && !inside && !gap)
}

// templateIcon is the one of the menu bar of the Mac: black on nothing, coloured by macOS.
func templateIcon() []byte {
	picture := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			if glyph(x, y) {
				picture.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	var out bytes.Buffer
	png.Encode(&out, picture)
	return out.Bytes()
}

// The sizes of the .ico file: Windows takes the one of its large icon (32 dots, 48 and 64 on a
// screen with bigger text) and shrinks it next to the clock.
var windowsSizes = []int{32, 48, 64}

// windowsIcon is the icon of the agent as an .ico file, one image per size.
func windowsIcon() []byte {
	source, err := png.Decode(bytes.NewReader(iconFile))
	if err != nil {
		logf("icon: %v", err)
		return nil
	}
	const header, entry, info = 6, 16, 40
	write := func(out *bytes.Buffer, values ...any) {
		for _, value := range values {
			binary.Write(out, binary.LittleEndian, value)
		}
	}
	var images [][]byte
	for _, size := range windowsSizes {
		picture := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(picture, picture.Bounds(), source, source.Bounds(), draw.Src, nil)

		var out bytes.Buffer
		// the height counts twice: the image and, below it, the mask of the transparent dots
		write(&out, uint32(info), int32(size), int32(size*2), uint16(1), uint16(32),
			uint32(0), uint32(size*size*4), int32(0), int32(0), uint32(0), uint32(0))
		for y := size - 1; y >= 0; y-- {
			for x := 0; x < size; x++ {
				dot := picture.NRGBAAt(x, y)
				out.Write([]byte{dot.B, dot.G, dot.R, dot.A})
			}
		}
		out.Write(make([]byte, ((size+31)/32)*4*size))
		images = append(images, out.Bytes())
	}

	var out bytes.Buffer
	write(&out, uint16(0), uint16(1), uint16(len(images)))
	offset := header + entry*len(images)
	for i, size := range windowsSizes {
		write(&out, uint8(size), uint8(size), uint8(0), uint8(0), uint16(1), uint16(32), uint32(len(images[i])), uint32(offset))
		offset += len(images[i])
	}
	for _, picture := range images {
		out.Write(picture)
	}
	return out.Bytes()
}
