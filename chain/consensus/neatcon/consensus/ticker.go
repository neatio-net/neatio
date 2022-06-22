package consensus

import (
	"sync"
	"time"

	"github.com/neatio-network/neatio/chain/log"

	. "github.com/neatlib/common-go"
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
	wg     sync.WaitGroup
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
	t.tickChan = make(chan timeoutInfo, tickTockBufferSize)
	t.tockChan = make(chan timeoutInfo, tickTockBufferSize)

	go t.timeoutRoutine()

	return nil
}

func (t *timeoutTicker) OnStop() {
	close(t.tickChan)
	close(t.tockChan)

	t.stopTimer()

	t.logger.Infof("timeoutTicker wait")
	t.wg.Wait()
	t.logger.Infof("timeoutTicker wait over")
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

	t.wg.Add(1)
	defer func() {
		t.wg.Done()
		t.logger.Infof("timeoutTicker done one routine")
	}()

	var ti timeoutInfo
	for {
		select {
		case newti := <-t.tickChan:
			// t.logger.Infof("Received tick. old_ti: %v, new_ti: %v", ti, newti)

			t.stopTimer()

			ti = newti
			t.timer.Reset(ti.Duration)
			//t.logger.Infof("Scheduled timeout. dur: %v, height: %v, round: %v, step: %v", ti.Duration, ti.Height, ti.Round, ti.Step)
		case <-t.timer.C:
			if !t.IsRunning() {
				//t.logger.Infof("timeoutTimer tickChan, but need stop or not running, just return")
				return
			}
			//t.logger.Infof("Timed out. dur: %v, height: %v, round: %v, step: %v", ti.Duration, ti.Height, ti.Round, ti.Step)
			go func(toi timeoutInfo) { t.tockChan <- toi }(ti)
		}
	}
}
