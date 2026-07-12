package extract

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/duffy1001/glean/llama"
)

var (
	backendInitOnce sync.Once
	backendFreeOnce sync.Once
	backendStarted  atomic.Bool
	backendStopped  atomic.Bool
)

func InitializeBackend() error {
	if backendStopped.Load() {
		return errors.New("llama backend has already been shut down")
	}
	backendInitOnce.Do(func() {
		llama.BackendInit()
		backendStarted.Store(true)
	})
	return nil
}

// ShutdownBackend releases process-global llama.cpp resources. No new Engine
// may be created after this function returns.
func ShutdownBackend() {
	backendFreeOnce.Do(func() {
		if backendStarted.Load() {
			llama.BackendFree()
			backendStopped.Store(true)
		}
	})
}
