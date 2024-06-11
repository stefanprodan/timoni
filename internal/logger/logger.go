/*
Copyright 2024 Stefan Prodan

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

package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/fluxcd/pkg/ssa"
	ssautil "github.com/fluxcd/pkg/ssa/utils"
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	gcrLog "github.com/google/go-containerregistry/pkg/logs"
	"github.com/rs/zerolog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtimeLog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NewConsoleLogger returns a human-friendly Logger.
// Pretty print adds timestamp, log level and colorized output to the logs.
func NewConsoleLogger(colorize, prettify bool) logr.Logger {
	color.NoColor = !colorize
	zconfig := zerolog.ConsoleWriter{Out: color.Error, NoColor: !colorize}
	if !prettify {
		zconfig.PartsExclude = []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
		}
	}

	zlog := zerolog.New(zconfig).With().Timestamp().Logger()

	// Discard the container registry client logger.
	gcrLog.Warn.SetOutput(io.Discard)

	// Create a logr.Logger using zerolog as sink.
	zerologr.VerbosityFieldName = ""
	log := zerologr.New(&zlog)

	// Set controller-runtime logger.
	runtimeLog.SetLogger(log)

	return log
}

var (
	colorDryRun       = color.New(color.FgHiBlack, color.Italic)
	colorError        = color.New(color.FgHiRed)
	colorReady        = color.New(color.FgHiGreen)
	colorCallerPrefix = color.New(color.FgHiBlack)
	colorBundle       = color.New(color.FgHiMagenta)
	colorInstance     = color.New(color.FgHiMagenta)
	colorPerAction    = map[ssa.Action]*color.Color{
		ssa.CreatedAction:    color.New(color.FgHiGreen),
		ssa.ConfiguredAction: color.New(color.FgHiCyan),
		ssa.UnchangedAction:  color.New(color.FgHiBlack),
		ssa.DeletedAction:    color.New(color.FgRed),
		ssa.SkippedAction:    color.New(color.FgHiBlack),
		ssa.UnknownAction:    color.New(color.FgYellow, color.Italic),
	}
	colorPerStatus = map[status.Status]*color.Color{
		status.InProgressStatus:  color.New(color.FgHiCyan, color.Italic),
		status.FailedStatus:      color.New(color.FgHiRed),
		status.CurrentStatus:     color.New(color.FgHiGreen),
		status.TerminatingStatus: color.New(color.FgRed),
		status.NotFoundStatus:    color.New(color.FgYellow, color.Italic),
		status.UnknownStatus:     color.New(color.FgYellow, color.Italic),
	}
)

type DryRunType string

const (
	DryRunClient DryRunType = "(dry run)"
	DryRunServer DryRunType = "(server dry run)"
)

func ColorizeJoin(values ...any) string {
	var sb strings.Builder
	for i, v := range values {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(ColorizeAny(v))
	}
	return sb.String()
}

func ColorizeAny(v any) string {
	switch v := v.(type) {
	case *unstructured.Unstructured:
		return ColorizeUnstructured(v)
	case DryRunType:
		return ColorizeDryRun(v)
	case ssa.Action:
		return ColorizeAction(v)
	case ssa.ChangeSetEntry:
		return ColorizeChangeSetEntry(v)
	case *ssa.ChangeSetEntry:
		return ColorizeChangeSetEntry(*v)
	case status.Status:
		return ColorizeStatus(v)
	case error:
		return ColorizeError(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func ColorizeSubject(subject string) string {
	return color.CyanString(subject)
}

func ColorizeReady(subject string) string {
	return colorReady.Sprint(subject)
}

func ColorizeInfo(subject string) string {
	return color.GreenString(subject)
}

func ColorizeWarning(subject string) string {
	return color.YellowString(subject)
}

func ColorizeUnstructured(object *unstructured.Unstructured) string {
	return ColorizeSubject(ssautil.FmtUnstructured(object))
}

func ColorizeAction(action ssa.Action) string {
	if c, ok := colorPerAction[action]; ok {
		return c.Sprint(action)
	}
	return action.String()
}

func ColorizeChange(subject string, action ssa.Action) string {
	return fmt.Sprintf("%s %s", ColorizeSubject(subject), ColorizeAction(action))
}

func ColorizeChangeSetEntry(change ssa.ChangeSetEntry) string {
	return ColorizeChange(change.Subject, change.Action)
}

func ColorizeDryRun(dryRun DryRunType) string {
	return colorDryRun.Sprint(string(dryRun))
}

func ColorizeError(err error) string {
	return colorError.Sprint(err.Error())
}

func ColorizeStatus(status status.Status) string {
	if c, ok := colorPerStatus[status]; ok {
		return c.Sprint(status)
	}
	return status.String()
}

func ColorizeBundle(bundle string) string {
	return colorCallerPrefix.Sprint("b:") + colorBundle.Sprint(bundle)
}

func ColorizeInstance(instance string) string {
	return colorCallerPrefix.Sprint("i:") + colorInstance.Sprint(instance)
}

func ColorizeRuntime(runtime string) string {
	return colorCallerPrefix.Sprint("r:") + colorInstance.Sprint(runtime)
}

func ColorizeCluster(cluster string) string {
	return colorCallerPrefix.Sprint("c:") + colorInstance.Sprint(cluster)
}

// StartSpinner starts a spinner with the given message.
func StartSpinner(msg string) interface{ Stop() } {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " " + msg
	s.Start()
	return s
}
