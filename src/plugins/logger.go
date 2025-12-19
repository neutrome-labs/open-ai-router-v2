package plugins

import (
	"go.uber.org/zap"
)

// Logger for plugin chain - can be set by modules
var Logger *zap.Logger = zap.NewNop()
