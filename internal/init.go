package internal

import (
	"net/url"
	"os"

	"github.com/WangYihang/Proxy-Verifier/internal/model"
	"github.com/google/uuid"
	flags "github.com/jessevdk/go-flags"
	logger "github.com/sirupsen/logrus"
)

// Init parses the command-line options and builds the shared runtime state.
// It returns them so the composition root (cmd/verifier) can pass them
// explicitly through the pipeline instead of relying on package-level globals.
func Init() (*model.MainOptions, *model.State) {
	options := &model.MainOptions{}
	// Parse options
	_, err := flags.ParseArgs(options, os.Args)
	if err != nil {
		os.Exit(1)
	}

	if options.MeasurementId == "" {
		options.MeasurementId = uuid.New().String()
	}

	// Validate options
	options.TargetUrlObject, err = url.ParseRequestURI(options.TargetUrl)
	if err != nil {
		os.Exit(1)
	}

	// Setup logger
	logger.SetFormatter(&logger.TextFormatter{
		FullTimestamp: true,
	})

	// Create state
	state := model.NewState(options.MonitorInterval)

	return options, state
}
