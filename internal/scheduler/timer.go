package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"stuff-time/internal/logger"
)

type Scheduler interface {
	Start(task func() error) error
	Stop() error
}

type FixedRateScheduler struct {
	interval time.Duration
	ticker   *time.Ticker
	done     chan bool
}

func NewFixedRateScheduler(interval time.Duration) *FixedRateScheduler {
	return &FixedRateScheduler{
		interval: interval,
		done:     make(chan bool),
	}
}

func (s *FixedRateScheduler) Start(task func() error) error {
	s.ticker = time.NewTicker(s.interval)
	
	go func() {
		for {
			select {
			case <-s.ticker.C:
				if err := task(); err != nil {
					logger.GetLogger().Errorf("Scheduled task execution failed: %v", err)
				}
			case <-s.done:
				return
			}
		}
	}()

	return nil
}

func (s *FixedRateScheduler) Stop() error {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.done)
	return nil
}

type CronScheduler struct {
	spec  string
	cron  *cron.Cron
	entry cron.EntryID
}

func NewCronScheduler(spec string) (*CronScheduler, error) {
	c := cron.New(cron.WithSeconds())
	return &CronScheduler{
		spec: spec,
		cron: c,
	}, nil
}

func (s *CronScheduler) Start(task func() error) error {
	entryID, err := s.cron.AddFunc(s.spec, func() {
		if err := task(); err != nil {
			logger.GetLogger().Errorf("Scheduled task execution failed: %v", err)
		}
	})
	if err != nil {
		return fmt.Errorf("invalid cron spec: %w", err)
	}

	s.entry = entryID
	s.cron.Start()
	return nil
}

func (s *CronScheduler) Stop() error {
	if s.cron != nil {
		s.cron.Stop()
	}
	return nil
}

func NewScheduler(interval string, cronSpec string) (Scheduler, error) {
	if cronSpec != "" {
		return NewCronScheduler(cronSpec)
	}

	if interval != "" {
		duration, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("invalid interval: %w", err)
		}
		return NewFixedRateScheduler(duration), nil
	}

	return nil, fmt.Errorf("either interval or cron must be specified")
}

