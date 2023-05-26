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
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	runtimeLog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NewConsoleLogger returns a human-friendly Logger.
// Pretty print adds timestamp, log level and colorized output to the logs.
func NewConsoleLogger(pretty bool) logr.Logger {
	output := zerolog.ConsoleWriter{Out: os.Stderr, NoColor: !pretty}
	if !pretty {
		output.PartsExclude = []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
		}
	}

	zlog := zerolog.New(output).With().Timestamp().Logger()

	zerologr.VerbosityFieldName = ""
	lg := zerologr.New(&zlog)
	runtimeLog.SetLogger(lg)

	return lg
}
