package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const databaseKeepaliveDefaultInterval = 6 * time.Hour

var databaseKeepaliveOnce sync.Once

func databaseKeepaliveIntervalFromEnv(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return databaseKeepaliveDefaultInterval
	}
	interval, err := time.ParseDuration(value)
	if err != nil || interval <= 0 {
		return databaseKeepaliveDefaultInterval
	}
	return interval
}

func StartDatabaseKeepaliveTask() {
	databaseKeepaliveOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		interval := databaseKeepaliveIntervalFromEnv(os.Getenv("DATABASE_KEEPALIVE_INTERVAL"))
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("database keepalive task started: interval=%s", interval))
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for range ticker.C {
				if err := model.PingDB(); err != nil {
					logger.LogWarn(context.Background(), fmt.Sprintf("database keepalive ping failed: %v", err))
				}
			}
		})
	})
}
