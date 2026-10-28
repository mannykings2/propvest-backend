package main

import (
	"context"
	"os"
	"time"

	"github.com/mannykings2/propvest-backend/internal/logger"
)

// heartbeatInterval is how often the file is refreshed. The container
// healthcheck (see docker-compose.yml) fails if the file is older than two
// intervals, so a wedged main loop — or a container that never started
// properly — shows up as unhealthy instead of as silent silence.
const heartbeatInterval = 30 * time.Second

// startHeartbeat touches a file on a ticker for the lifetime of the process.
//
// WHY A FILE: the worker has no HTTP port (it is a consumer, not a server), so
// there is nothing for a container healthcheck to poll. The file is the
// cheapest probe that works inside a bare alpine image.
func startHeartbeat(ctx context.Context) func() {
	path := os.Getenv("WORKER_HEARTBEAT_FILE")
	if path == "" {
		path = "/tmp/propvest-worker-heartbeat"
	}

	touch := func() {
		if err := os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
			logger.FromContext(ctx).Error("failed to write heartbeat file", "path", path, "error", err)
		}
	}
	touch() // so the healthcheck passes immediately after a clean start

	ticker := time.NewTicker(heartbeatInterval)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				touch()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	return func() { close(done) }
}
