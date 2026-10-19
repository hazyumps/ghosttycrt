import AppKit
import Darwin

// Stay in this app bundle when starting the terminal. Opening Ghostty.app
// would reuse its Dock identity and spread our custom icon to its instances.
func fail(_ message: String) -> Never {
    let alert = NSAlert()
    alert.messageText = "GCRT could not open"
    alert.informativeText = message
    alert.alertStyle = .critical
    alert.runModal()
    exit(1)
}

let home = FileManager.default.homeDirectoryForCurrentUser.path
let path = "\(home)/go/bin:\(home)/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:"
    + (ProcessInfo.processInfo.environment["PATH"] ?? "")
setenv("PATH", path, 1)
for command in ["gcrt", "tmux"] {
    let found = path.split(separator: ":").contains {
        FileManager.default.isExecutableFile(atPath: "\($0)/\(command)")
    }
    if !found { fail("Install \(command) in ~/go/bin, ~/.local/bin, or a Homebrew bin directory.") }
}

guard let resources = Bundle.main.resourceURL else { fail("The app resources are missing. Rebuild GCRT.app.") }
let terminal = Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/ghostty").path
guard FileManager.default.isExecutableFile(atPath: terminal) else { fail("The bundled terminal is missing. Rebuild GCRT.app.") }
let arguments = [
    terminal,
    "--title=GCRT",
    "--macos-icon=custom",
    "--macos-custom-icon=\(resources.appendingPathComponent("GCRT.icns").path)",
    "--auto-update=off",
    "--window-save-state=never",
    "--working-directory=\(home)",
    // Every new terminal window attaches to the existing GCRT workspace.
    "--command=\(resources.appendingPathComponent("start-session").path)",
    "--quit-after-last-window-closed=true",
]
var argv = arguments.map { strdup($0) } + [nil]
argv.withUnsafeMutableBufferPointer { buffer in
    _ = execv(terminal, buffer.baseAddress!)
}
fail("The bundled terminal could not start: \(String(cString: strerror(errno)))")
