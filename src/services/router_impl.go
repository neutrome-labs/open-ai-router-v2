package services

import (
	"sync"

	"go.uber.org/zap"
)

type RouterImpl struct {
	Name        string
	AuthManager AuthManager
	Mu          sync.RWMutex
	Logger      *zap.Logger
}
