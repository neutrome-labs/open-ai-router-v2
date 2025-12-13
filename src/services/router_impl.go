package services

import (
	"sync"

	"go.uber.org/zap"
)

// RouterImpl provides the runtime implementation for a router
type RouterImpl struct {
	Name        string
	AuthManager AuthManager
	Mu          sync.RWMutex
	Logger      *zap.Logger
}
