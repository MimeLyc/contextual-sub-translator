package icron

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

type TriggerInfo struct {
	Next       time.Time
	Last       time.Time
	Expression string

	TimeSinceLast time.Duration
	TimeUntilNext time.Duration
}

func GetTriggerInfo(cronExpr string, refTime time.Time) (*TriggerInfo, error) {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour |
		cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	nextTime := schedule.Next(refTime)

	var prevTime time.Time
	searchStart := refTime.Add(-time.Minute)

	for i := range 366 * 24 {
		checkTime := searchStart.Add(-time.Duration(i) * time.Hour)
		candidateNext := schedule.Next(checkTime)

		if candidateNext.Before(refTime) ||
			candidateNext.Equal(refTime) {
			prevTime = candidateNext
			break
		}
	}

	info := &TriggerInfo{
		Expression: cronExpr,
		Next:       nextTime,
		Last:       prevTime,
	}

	if !prevTime.IsZero() {
		info.TimeSinceLast = refTime.Sub(prevTime)
	}

	info.TimeUntilNext = nextTime.Sub(refTime)

	return info, nil
}
