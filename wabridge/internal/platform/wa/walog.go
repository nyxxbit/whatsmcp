package wa

import (
	"fmt"

	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// waLogAdapter conecta o logger do whatsmeow (printf) ao nosso ports.Logger
// (estruturado + rotação). Mantém o log enxuto como o nível WARN do bridge legado:
// Info/Debug são descartados; só Warn/Error passam adiante (fim do log gigante).
type waLogAdapter struct {
	log    ports.Logger
	module string
}

var _ waLog.Logger = (*waLogAdapter)(nil)

func newWALog(log ports.Logger, module string) *waLogAdapter {
	return &waLogAdapter{log: log, module: module}
}

func (a *waLogAdapter) Warnf(msg string, args ...interface{}) {
	a.log.Warn(fmt.Sprintf(msg, args...), "wa_module", a.module)
}

func (a *waLogAdapter) Errorf(msg string, args ...interface{}) {
	a.log.Error(fmt.Sprintf(msg, args...), "wa_module", a.module)
}

func (a *waLogAdapter) Infof(string, ...interface{})  {} // descartado (log enxuto)
func (a *waLogAdapter) Debugf(string, ...interface{}) {} // descartado (log enxuto)

func (a *waLogAdapter) Sub(module string) waLog.Logger {
	return &waLogAdapter{log: a.log, module: a.module + "/" + module}
}
