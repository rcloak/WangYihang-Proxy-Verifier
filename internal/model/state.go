package model

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
)

type State struct {
	MonitorInterval int `json:"monitor_interval"`

	// processed counts tasks that have finished; it is updated concurrently by
	// every worker, so it is an atomic counter rather than a plain field.
	processed         atomic.Int64
	previousProcessed int64

	NumScheduledTasks int `json:"num_scheduled_tasks"`
	NumFinishedTasks  int `json:"num_finished_tasks"`
	NumFailedTasks    int `json:"num_failed_tasks"`
	NumSucceedTasks   int `json:"num_succeed_tasks"`

	lock *sync.Mutex `json:"-"`
}

func NewState(monitorInterval int) *State {
	return &State{
		MonitorInterval: monitorInterval,

		NumScheduledTasks: 0,
		NumFinishedTasks:  0,
		NumFailedTasks:    0,
		NumSucceedTasks:   0,

		lock: &sync.Mutex{},
	}
}

func (s *State) String() string {
	var m runtime.MemStats

	s.lock.Lock()
	numScheduled := s.NumScheduledTasks
	numFinished := s.NumFinishedTasks
	numFailed := s.NumFailedTasks
	numSucceed := s.NumSucceedTasks
	s.lock.Unlock()

	processed := s.processed.Load()
	scheduleSpeed := float64(processed-s.previousProcessed) / float64(s.MonitorInterval)

	var hitRate float64
	if numSucceed+numFailed > 0 {
		hitRate = float64(numSucceed) / float64(numSucceed+numFailed)
	}

	runtime.ReadMemStats(&m)
	return fmt.Sprintf(
		"[%s] processed: %s, schedule speed: %s/s, %s failed, %s succeed, %s / %s overall, hitrate: %s, memory usage: %s, go routine number: %s",
		color.New(color.FgYellow).Sprint(time.Now().Format("2006-01-02 15:04:05")),
		color.New(color.FgGreen).Sprint(processed),
		color.New(color.FgBlue).Sprintf("%.2f", scheduleSpeed),
		color.New(color.FgRed).Sprint(numFailed),
		color.New(color.FgGreen).Sprint(numSucceed),
		color.New(color.FgYellow).Sprint(numFinished),
		color.New(color.FgYellow).Sprint(numScheduled),
		color.New(color.FgCyan).Sprintf("%.2f%%", hitRate*100),
		color.New(color.FgYellow).Sprint(humanize.Bytes(m.Alloc)),
		color.New(color.FgYellow).Sprint(runtime.NumGoroutine()),
	)
}

func (s *State) TaskScheduled() {
	s.lock.Lock()
	s.NumScheduledTasks += 1
	s.lock.Unlock()
}

func (s *State) TaskDone(isSucceed bool) {
	s.processed.Add(1)
	s.lock.Lock()
	s.NumFinishedTasks += 1
	if isSucceed {
		s.NumSucceedTasks += 1
	} else {
		s.NumFailedTasks += 1
	}
	s.lock.Unlock()
}

func (s *State) Snapshot() {
	fmt.Println(s.String())
	s.previousProcessed = s.processed.Load()
}
