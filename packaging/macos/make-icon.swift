import AppKit

// Code-drawn icon, so the launcher can be rebuilt without external artwork.
let output = CommandLine.arguments[1]
let side = 1024
let bitmap = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: side, pixelsHigh: side,
    bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
    colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
NSGraphicsContext.saveGraphicsState()
NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)

NSColor(calibratedRed: 0.11, green: 0.13, blue: 0.16, alpha: 1).setFill()
NSBezierPath(roundedRect: NSRect(x: 56, y: 56, width: 912, height: 912),
             xRadius: 210, yRadius: 210).fill()
let green = NSColor(calibratedRed: 0.50, green: 0.88, blue: 0.65, alpha: 1)
green.setStroke()
let prompt = NSBezierPath()
prompt.lineWidth = 42
prompt.lineCapStyle = .round
prompt.lineJoinStyle = .round
prompt.move(to: NSPoint(x: 240, y: 730))
prompt.line(to: NSPoint(x: 350, y: 630))
prompt.line(to: NSPoint(x: 240, y: 530))
prompt.move(to: NSPoint(x: 430, y: 530))
prompt.line(to: NSPoint(x: 580, y: 530))
prompt.stroke()

let text = "GCRT" as NSString
let attrs: [NSAttributedString.Key: Any] = [
    .font: NSFont.monospacedSystemFont(ofSize: 180, weight: .bold),
    .foregroundColor: NSColor.white
]
let size = text.size(withAttributes: attrs)
text.draw(at: NSPoint(x: (1024 - size.width) / 2, y: 230), withAttributes: attrs)
NSGraphicsContext.restoreGraphicsState()
try bitmap.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: output))
