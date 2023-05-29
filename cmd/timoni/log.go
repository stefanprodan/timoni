/*
Copyright 2023 Stefan Prodan

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	gcrLog "github.com/google/go-containerregistry/pkg/logs"
	"github.com/rs/zerolog"
	runtimeLog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NewConsoleLogger returns a human-friendly Logger.
// Pretty print adds timestamp, log level and colorized output to the logs.
func NewConsoleLogger(pretty bool) logr.Logger {
	zconfig := zerolog.ConsoleWriter{Out: os.Stderr, NoColor: !pretty}
	if !pretty {
		zconfig.PartsExclude = []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
		}
	}

	zlog := zerolog.New(zconfig).With().Timestamp().Logger()

	// Set container registry client logger.
	gcrLog.Warn.SetOutput(zlog)

	// Create a logr.Logger using zerolog as sink.
	zerologr.VerbosityFieldName = ""
	log := zerologr.New(&zlog)

	// Set controller-runtime logger.
	runtimeLog.SetLogger(log)

	return log
}

// LoggerFrom returns a logr.Logger with predefined values from a context.Context.
func LoggerFrom(ctx context.Context, keysAndValues ...interface{}) logr.Logger {
	newLogger := logger
	if ctx != nil {
		if l, err := logr.FromContext(ctx); err == nil {
			newLogger = l
		}
	}
	return newLogger.WithValues(keysAndValues...)
}

// StartSpinner starts a spinner with the given message.
func StartSpinner(msg string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " " + msg
	s.Start()
	return s
}
