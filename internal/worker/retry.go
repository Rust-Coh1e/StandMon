package worker

import (
	"time"
)

func RetryDelay(attempt int) (bool, time.Duration) {
	switch {
	case attempt == 0:
		return true, 10 * time.Second
	case attempt == 1:
		return true, 30 * time.Second
	case attempt == 2:
		return true, 60 * time.Second
	case attempt == 3:
		return true, 120 * time.Second
	case attempt >= 4:
		return false, 0 * time.Second
	}

	return false, 0 * time.Second
}
