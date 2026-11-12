package knowledge

import (
	"errors"
	"site-assistant/internal/core"
)

func importError(e error) error {
	var f *core.Fault
	if errors.As(e, &f) {
		return f
	}
	return core.Fail("import_failed", 503)
}
