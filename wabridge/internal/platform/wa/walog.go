package wa

import (
	"fmt"

	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// waLogAdapter connects whatsmeow's logger (printf-style) to our ports.Logger
// (structured + rotating). Keeps logging lean, matching the legacy bridge's
// WARN level: Info/Debug are discarded; only Warn/Error get passed through
// (no more giant log files).
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

func (a *waLogAdapter) Infof(string, ...interface{})  {} // discarded (lean logging)
func (a *waLogAdapter) Debugf(string, ...interface{}) {} // discarded (lean logging)

func (a *waLogAdapter) Sub(module string) waLog.Logger {
	return &waLogAdapter{log: a.log, module: a.module + "/" + module}
}
