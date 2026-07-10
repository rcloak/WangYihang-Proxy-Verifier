package main

import (
	"github.com/WangYihang/Proxy-Verifier/internal"
	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/WangYihang/Proxy-Verifier/internal/processer"
	logger "github.com/sirupsen/logrus"
)

func main() {
	options, state := internal.Init()

	// Create task queue and result queue
	var taskQueue = make(chan *model.Task, options.NumWorkers)
	var resultQueue = make(chan *model.Result)

	// Start loader
	logger.Info("Starting loader...")
	go processer.Loader(taskQueue, options, state)

	// Start workers
	logger.Infof("Starting %d workers...", options.NumWorkers)
	for i := 0; i < options.NumWorkers; i++ {
		go processer.Worker(taskQueue, resultQueue)
	}

	// Start monitor
	go processer.Monitor(options.MonitorInterval, state)

	// Start saver
	logger.Info("Starting saver...")
	processer.Saver(resultQueue, options.NumWorkers, options.OutputFile, state)

	// Print statistics
	state.Snapshot()
}
