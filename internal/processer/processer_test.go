package processer_test

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WangYihang/Proxy-Verifier/internal"
	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/processer"
)

func setupOptions(t *testing.T, targetURL, outputFile string) {
	t.Helper()
	obj, err := url.ParseRequestURI(targetURL)
	if err != nil {
		t.Fatalf("bad target url: %v", err)
	}
	internal.Options = &model.MainOptions{
		TargetUrl:       targetURL,
		TargetUrlObject: obj,
		MeasurementId:   "test-measurement",
		Timeout:         8,
		OutputFile:      outputFile,
	}
	internal.State = model.NewState(1)
}

func TestScheduleTask(t *testing.T) {
	internal.State = model.NewState(1)
	queue := make(chan *model.Task, 1)
	task := model.NewTask(0, "m", "http", "1.2.3.4", 80, "http://e/", 1, nil)

	processer.ScheduleTask(&task, queue)

	got := <-queue
	if got.ProxyHost != "1.2.3.4" {
		t.Errorf("scheduled task host = %q, want 1.2.3.4", got.ProxyHost)
	}
	if internal.State.NumScheduledTasks != 1 {
		t.Errorf("NumScheduledTasks = %d, want 1", internal.State.NumScheduledTasks)
	}
}

func TestWorkerProcessesTaskAndSignalsCompletion(t *testing.T) {
	taskQueue := make(chan *model.Task, 2)
	resultQueue := make(chan *model.Result, 4)

	processed := false
	stub := func(tk *model.Task) model.Result {
		processed = true
		return model.Result{Task: tk}
	}
	task := model.NewTask(0, "m", "http", "1.2.3.4", 8080, "http://e/", 1, stub)

	taskQueue <- &task
	taskQueue <- nil // tells the worker to stop

	go processer.Worker(taskQueue, resultQueue)

	result := <-resultQueue
	if result == nil {
		t.Fatal("expected a result, got the completion sentinel")
	}
	if !processed {
		t.Error("the task's ProcessFunc was not called")
	}
	if result.Task.ProxyHost != "1.2.3.4" {
		t.Errorf("result host = %q, want 1.2.3.4", result.Task.ProxyHost)
	}

	if sentinel := <-resultQueue; sentinel != nil {
		t.Error("expected a nil completion sentinel after the worker stops")
	}
}

func TestLoaderCreatesTasksFromCsv(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "proxies.csv")
	if err := os.WriteFile(input, []byte("http,1.2.3.4,8080\nsocks5,9.9.9.9,1080\n"), 0644); err != nil {
		t.Fatal(err)
	}
	setupOptions(t, "http://echo.example/", filepath.Join(dir, "out.log"))

	const numWorkers = 2
	taskQueue := make(chan *model.Task, 64)
	go processer.Loader(input, taskQueue, numWorkers)

	var tasks []*model.Task
	sentinels := 0
	for sentinels < numWorkers {
		task := <-taskQueue
		if task == nil {
			sentinels++
			continue
		}
		tasks = append(tasks, task)
	}

	// For an http target: an http proxy expands to 3 techniques
	// (proxy, transparent_proxy, tunnel) and a socks5 proxy to 1 (tunnel).
	if len(tasks) != 4 {
		t.Fatalf("got %d tasks, want 4", len(tasks))
	}
	byProtocol := map[string]int{}
	for _, task := range tasks {
		byProtocol[task.ProxyProtocol]++
		if task.ProcessFunc == nil {
			t.Errorf("task for %s has no ProcessFunc", task.ProxyProtocol)
		}
	}
	if byProtocol["http"] != 3 {
		t.Errorf("http techniques = %d, want 3", byProtocol["http"])
	}
	if byProtocol["socks5"] != 1 {
		t.Errorf("socks5 techniques = %d, want 1", byProtocol["socks5"])
	}
	if internal.State.NumScheduledTasks != 4 {
		t.Errorf("NumScheduledTasks = %d, want 4", internal.State.NumScheduledTasks)
	}
}

func TestLoaderMissingFile(t *testing.T) {
	setupOptions(t, "http://echo.example/", filepath.Join(t.TempDir(), "out.log"))
	const numWorkers = 3
	taskQueue := make(chan *model.Task, 8)

	// A missing input file must still release the workers via nil sentinels
	// rather than hanging or panicking.
	go processer.Loader(filepath.Join(t.TempDir(), "nope.csv"), taskQueue, numWorkers)

	sentinels := 0
	for sentinels < numWorkers {
		if task := <-taskQueue; task == nil {
			sentinels++
		}
	}
}

func TestSaverWritesResults(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.log")
	setupOptions(t, "http://echo.example/", out)

	task := model.NewTask(0, "m", "http", "1.2.3.4", 8080, "http://e/", 1, nil)
	resultQueue := make(chan *model.Result, 8)
	resultQueue <- &model.Result{Task: &task}
	resultQueue <- &model.Result{Task: &task, Error: "connection refused"}
	resultQueue <- nil // single worker finished

	processer.Saver(resultQueue, 1)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("output has %d lines, want 2:\n%s", len(lines), string(data))
	}

	var first model.Result
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("output line is not valid JSON: %v", err)
	}
	if first.Task.ProxyHost != "1.2.3.4" {
		t.Errorf("serialized host = %q, want 1.2.3.4", first.Task.ProxyHost)
	}

	if internal.State.NumSucceedTasks != 1 {
		t.Errorf("NumSucceedTasks = %d, want 1", internal.State.NumSucceedTasks)
	}
	if internal.State.NumFailedTasks != 1 {
		t.Errorf("NumFailedTasks = %d, want 1", internal.State.NumFailedTasks)
	}
}
