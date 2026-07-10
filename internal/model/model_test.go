package model_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
)

func dummyProcess(*model.Task) model.Result { return model.Result{} }

func TestNewTaskPopulatesFields(t *testing.T) {
	task := model.NewTask(7, "measurement-1", "http", "1.2.3.4", 8080, "http://example.com/", 16, dummyProcess)

	if task.Index != 7 {
		t.Errorf("Index = %d, want 7", task.Index)
	}
	if task.MeasurementId != "measurement-1" {
		t.Errorf("MeasurementId = %q, want measurement-1", task.MeasurementId)
	}
	if task.ProxyProtocol != "http" {
		t.Errorf("ProxyProtocol = %q, want http", task.ProxyProtocol)
	}
	if task.ProxyHost != "1.2.3.4" {
		t.Errorf("ProxyHost = %q, want 1.2.3.4", task.ProxyHost)
	}
	if task.ProxyPort != 8080 {
		t.Errorf("ProxyPort = %d, want 8080", task.ProxyPort)
	}
	if task.TargetUri != "http://example.com/" {
		t.Errorf("TargetUri = %q, want http://example.com/", task.TargetUri)
	}
	if task.Timeout != 16 {
		t.Errorf("Timeout = %d, want 16", task.Timeout)
	}
	if task.TaskId == "" {
		t.Error("TaskId should be generated, got empty string")
	}
	if task.ProcessFunc == nil {
		t.Error("ProcessFunc should be set")
	}
	if task.ProxyUri != "http://1.2.3.4:8080" {
		t.Errorf("ProxyUri = %q, want http://1.2.3.4:8080", task.ProxyUri)
	}
}

func TestNewTaskProxyURI(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		timeout  int
		wantURI  string
	}{
		{"http no timeout suffix", "http", 8, "http://1.2.3.4:1080"},
		{"https no timeout suffix", "https", 8, "https://1.2.3.4:1080"},
		{"socks5 gets timeout suffix", "socks5", 8, "socks5://1.2.3.4:1080?timeout=8s"},
		{"socks4 gets timeout suffix", "socks4", 5, "socks4://1.2.3.4:1080?timeout=5s"},
		{"socks5 zero timeout no suffix", "socks5", 0, "socks5://1.2.3.4:1080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := model.NewTask(0, "m", tt.protocol, "1.2.3.4", 1080, "http://e/", tt.timeout, dummyProcess)
			if task.ProxyUri != tt.wantURI {
				t.Errorf("ProxyUri = %q, want %q", task.ProxyUri, tt.wantURI)
			}
		})
	}
}

func TestNewTaskGeneratesUniqueIDs(t *testing.T) {
	a := model.NewTask(0, "m", "http", "1.2.3.4", 80, "http://e/", 1, dummyProcess)
	b := model.NewTask(0, "m", "http", "1.2.3.4", 80, "http://e/", 1, dummyProcess)
	if a.TaskId == b.TaskId {
		t.Errorf("task ids should be unique, both were %q", a.TaskId)
	}
}

func TestResultString(t *testing.T) {
	task := model.NewTask(3, "m", "socks5", "9.9.9.9", 1080, "http://e/", 1, dummyProcess)

	success := model.Result{Task: &task}
	if got := success.String(); !strings.Contains(got, "succeed") {
		t.Errorf("successful result string = %q, want it to contain 'succeed'", got)
	}

	failure := model.Result{Task: &task, Error: "connection refused"}
	got := failure.String()
	if !strings.Contains(got, "failed") {
		t.Errorf("failed result string = %q, want it to contain 'failed'", got)
	}
	if !strings.Contains(got, "socks5://9.9.9.9:1080") {
		t.Errorf("result string = %q, want it to contain the proxy address", got)
	}
}

func TestStateCounters(t *testing.T) {
	s := model.NewState(1)
	s.TaskScheduled()
	s.TaskScheduled()
	s.TaskScheduled()
	s.TaskDone(true)
	s.TaskDone(true)
	s.TaskDone(false)

	if s.NumScheduledTasks != 3 {
		t.Errorf("NumScheduledTasks = %d, want 3", s.NumScheduledTasks)
	}
	if s.NumFinishedTasks != 3 {
		t.Errorf("NumFinishedTasks = %d, want 3", s.NumFinishedTasks)
	}
	if s.NumSucceedTasks != 2 {
		t.Errorf("NumSucceedTasks = %d, want 2", s.NumSucceedTasks)
	}
	if s.NumFailedTasks != 1 {
		t.Errorf("NumFailedTasks = %d, want 1", s.NumFailedTasks)
	}
}

// TestStateStringNoDivideByZero exercises the hit-rate formatting on a fresh
// state where no task has finished yet; it must not produce NaN.
func TestStateStringNoDivideByZero(t *testing.T) {
	s := model.NewState(1)
	out := s.String()
	if strings.Contains(out, "NaN") {
		t.Errorf("fresh state string must not contain NaN, got %q", out)
	}
}

// TestStateConcurrentTaskDone runs many concurrent TaskDone calls; combined with
// `go test -race` this guards against the data race the counters used to have.
func TestStateConcurrentTaskDone(t *testing.T) {
	s := model.NewState(1)
	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			s.TaskDone(i%2 == 0)
		}(i)
	}
	wg.Wait()

	if s.NumFinishedTasks != n {
		t.Errorf("NumFinishedTasks = %d, want %d", s.NumFinishedTasks, n)
	}
	if s.NumSucceedTasks+s.NumFailedTasks != n {
		t.Errorf("succeed+failed = %d, want %d", s.NumSucceedTasks+s.NumFailedTasks, n)
	}
	if s.NumSucceedTasks != n/2 {
		t.Errorf("NumSucceedTasks = %d, want %d", s.NumSucceedTasks, n/2)
	}
}

// TestStateSnapshotDoesNotPanic makes sure Snapshot (which reads the atomic
// progress counter) is safe to call.
func TestStateSnapshotDoesNotPanic(t *testing.T) {
	s := model.NewState(1)
	s.TaskDone(true)
	s.Snapshot()
}
