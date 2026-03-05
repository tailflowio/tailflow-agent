package server

import (
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// CronScheduler wraps robfig/cron for scheduled workflow execution.
type CronScheduler struct {
	cron   *cron.Cron
	logger *slog.Logger
}

func NewCronScheduler(logger *slog.Logger) *CronScheduler {
	return &CronScheduler{
		cron:   cron.New(),
		logger: logger,
	}
}

func (cs *CronScheduler) Add(spec string, fn func()) error {
	_, err := cs.cron.AddFunc(spec, fn)
	if err != nil {
		return err
	}

	cs.logger.Info("cron job added", "spec", spec)

	return nil
}

func (cs *CronScheduler) Start() {
	cs.cron.Start()
	cs.logger.Info("cron scheduler started")
}

func (cs *CronScheduler) Stop() {
	ctx := cs.cron.Stop()
	<-ctx.Done()
	cs.logger.Info("cron scheduler stopped")
}

func NextRun(spec string) *time.Time {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

	sched, err := parser.Parse(spec)
	if err != nil {
		return nil
	}

	next := sched.Next(time.Now())

	return &next
}
