package tray

import "os/exec"

// openExternal abre um arquivo ou URL no app padrão do Windows (navegador para
// URLs, visualizador para imagens). url.dll,FileProtocolHandler trata ambos.
func openExternal(target string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
}
