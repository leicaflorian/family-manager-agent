// The icon of the app of the Mac, 1024 dots, from the icon of the agent: build.sh runs it.
//
//   swift mac-icon.swift <icon.png> <out.png>
//
// macOS wants its own shape: the drawing in a square of 824 dots in the middle of 1024, with the
// corners rounded by 185.4. An icon that does not fit that shape (the square of Family Manager
// has sharper corners) is shown small on a grey plate by macOS 26, and too big by the ones
// before. The background of the icon is one colour, so the corners are filled with it.
import AppKit

let arguments = CommandLine.arguments
guard arguments.count == 3, let source = NSImage(contentsOfFile: arguments[1]),
      let dots = NSBitmapImageRep(data: source.tiffRepresentation ?? Data()) else {
    FileHandle.standardError.write("usage: swift mac-icon.swift <icon.png> <out.png>\n".data(using: .utf8)!)
    exit(1)
}

let size = 1024
let body = NSRect(x: 100, y: 100, width: 824, height: 824)
let picture = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size, bitsPerSample: 8,
                               samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB,
                               bytesPerRow: 0, bitsPerPixel: 0)!
picture.size = NSSize(width: size, height: size)

NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: picture)
NSGraphicsContext.current!.imageInterpolation = .high
NSBezierPath(roundedRect: body, xRadius: 185.4, yRadius: 185.4).addClip()
// the colour of the background: the dot in the middle of the top edge
dots.colorAt(x: dots.pixelsWide / 2, y: 2)!.setFill()
body.fill()
source.draw(in: body)
NSGraphicsContext.restoreGraphicsState()

try! picture.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: arguments[2]))
