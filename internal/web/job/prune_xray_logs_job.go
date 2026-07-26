package job

import (
	"os"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const defaultMaxXrayLogBytes int64 = 64 << 20

var maxXrayLogBytes = defaultMaxXrayLogBytes

// PruneXrayLogsJob truncates Xray access and error logs once either exceeds maxXrayLogBytes.
type PruneXrayLogsJob struct{}

// NewPruneXrayLogsJob creates a new Xray log pruning job instance.
func NewPruneXrayLogsJob() *PruneXrayLogsJob {
	return new(PruneXrayLogsJob)
}

func (j *PruneXrayLogsJob) Run() {
	truncateXrayLog(xray.GetAccessLogPath, maxXrayLogBytes)
	truncateXrayLog(xray.GetErrorLogPath, maxXrayLogBytes)
}

func truncateXrayLog(pathFn func() (string, error), maxBytes int64) {
	logPath, err := pathFn()
	if err != nil || disabledLogPath(logPath) {
		return
	}
	if maxBytes > 0 {
		info, err := os.Stat(logPath)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.Warning("Failed to stat Xray log:", logPath, "-", err)
			}
			return
		}
		if info.Size() <= maxBytes {
			return
		}
	}
	if err := os.Truncate(logPath, 0); err != nil && !os.IsNotExist(err) {
		logger.Warning("Failed to truncate Xray log:", logPath, "-", err)
	}
}

func disabledLogPath(path string) bool {
	return path == "" || path == "none"
}
