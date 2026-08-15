package tray

import "os/exec"

// openExternal opens a file or URL in the default Windows app (browser for
// URLs, viewer for images). url.dll,FileProtocolHandler handles both.
func openExternal(target string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
}
