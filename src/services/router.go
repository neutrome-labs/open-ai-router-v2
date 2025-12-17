package services

import (
	"sync"

	"go.uber.org/zap"
)

// RouterService provides the runtime implementation for a router
type RouterService struct {
	Name   string
	Auth   AuthService
	Mu     sync.RWMutex
	Logger *zap.Logger
}
