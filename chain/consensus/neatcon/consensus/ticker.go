package consensus

import (
	"time"

	"github.com/nio-net/nio/chain/log"

	. "github.com/nio-net/common"
)

var (
	tickTockBufferSize = 10
)

type TimeoutTicker interface {
	Start() (bool, error)
	Stop() bool
	Chan() <-chan timeoutInfo
	ScheduleTimeout(ti timeoutInfo)
}

type timeoutTicker struct {
	BaseService

	timer    *time.Timer
	tickChan chan timeoutInfo
	tockChan chan timeoutInfo

	logger log.Logger
}

func NewTimeoutTicker(logger log.Logger) TimeoutTicker {
	tt := &timeoutTicker{
		timer:    time.NewTimer(0),
		tickChan: make(chan timeoutInfo, tickTockBufferSize),
		tockChan: make(chan timeoutInfo, tickTockBufferSize),
		logger:   logger,
	}
	tt.stopTimer()
	tt.BaseService = *NewBaseService(logger, "TimeoutTicker", tt)
	return tt
}

func (t *timeoutTicker) OnStart() error {

	go t.timeoutRoutine()

	return nil
}

func (t *timeoutTicker) OnStop() {
	t.BaseService.OnStop()
	t.stopTimer()
}

func (t *timeoutTicker) Chan() <-chan timeoutInfo {
	return t.tockChan
}

func (t *timeoutTicker) ScheduleTimeout(ti timeoutInfo) {
	t.tickChan <- ti
}

func (t *timeoutTicker) stopTimer() {

	if !t.timer.Stop() {
		select {
		case <-t.timer.C:
		default:
			t.logger.Debug("Timer already stopped")
		}
	}
}

func (t *timeoutTicker) timeoutRoutine() {
	t.logger.Info("Starting timeout routine")
	var ti timeoutInfo
	for {
		select {
		case newti := <-t.tickChan:

			t.stopTimer()

			ti = newti
			t.timer.Reset(ti.Duration)

		case <-t.timer.C:

			go func(toi timeoutInfo) { t.tockChan <- toi }(ti)
		case <-t.Quit:
			return
		}
	}
}
