package config

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

func initLogger(cfg BaseConfig) {
	// Parse Log Level
	logLevel, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		logLevel = log.InfoLevel
	}
	log.SetLevel(logLevel)

	// Create Log Formatter
	var logFormatter log.Formatter
	switch cfg.LogFormat {
	case "json":
		log.SetReportCaller(true)
		logFormatter = &log.JSONFormatter{
			TimestampFormat:  time.RFC3339,
			CallerPrettyfier: prettyCaller,
		}
	case "text":
		logFormatter = &log.TextFormatter{
			TimestampFormat: time.RFC3339,
			FullTimestamp:   true,
		}
	}

	log.SetFormatter(&utcFormatter{logFormatter})
}

func prettyCaller(f *runtime.Frame) (function string, file string) {
	purgePrefix := "reichard.io/conduit/"

	pathName := strings.Replace(f.Func.Name(), purgePrefix, "", 1)
	parts := strings.Split(pathName, ".")

	filepath, line := f.Func.FileLine(f.PC)
	splitFilePath := strings.Split(filepath, "/")

	fileName := fmt.Sprintf("%s/%s@%d", parts[0], splitFilePath[len(splitFilePath)-1], line)
	functionName := strings.Replace(pathName, parts[0]+".", "", 1)

	return functionName, fileName
}

type utcFormatter struct {
	log.Formatter
}

func (cf utcFormatter) Format(e *log.Entry) ([]byte, error) {
	e.Time = e.Time.UTC()
	return cf.Formatter.Format(e)
}
