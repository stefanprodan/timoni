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
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	gcrLog "github.com/google/go-containerregistry/pkg/logs"
	"github.com/rs/zerolog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtimeLog "sigs.k8s.io/controller-runtime/pkg/log"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// NewConsoleLogger returns a human-friendly Logger.
// Pretty print adds timestamp, log level and colorized output to the logs.
func NewConsoleLogger() logr.Logger {
	color.NoColor = !rootArgs.coloredLog
	zconfig := zerolog.ConsoleWriter{Out: color.Error, NoColor: !rootArgs.coloredLog}
	if !rootArgs.prettyLog {
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

type dryRunType string

const (
	dryRunClient dryRunType = "(dry run)"
	dryRunServer dryRunType = "(server dry run)"
)

func colorizeJoin(values ...any) string {
	var sb strings.Builder
	for i, v := range values {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(colorizeAny(v))
	}
	return sb.String()
}

func colorizeAny(v any) string {
	switch v := v.(type) {
	case *unstructured.Unstructured:
		return colorizeUnstructured(v)
	case dryRunType:
		return colorizeDryRun(v)
	case ssa.Action:
		return colorizeAction(v)
	case ssa.ChangeSetEntry:
		return colorizeChangeSetEntry(v)
	case *ssa.ChangeSetEntry:
		return colorizeChangeSetEntry(*v)
	case status.Status:
		return colorizeStatus(v)
	case error:
		return colorizeError(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func colorizeSubject(subject string) string {
	return color.CyanString(subject)
}

func colorizeInfo(subject string) string {
	return color.GreenString(subject)
}

func colorizeWarning(subject string) string {
	return color.YellowString(subject)
}

func colorizeNamespaceFromArgs() string {
	return colorizeSubject("Namespace/" + *kubeconfigArgs.Namespace)
}

func colorizeUnstructured(object *unstructured.Unstructured) string {
	return colorizeSubject(ssa.FmtUnstructured(object))
}

func colorizeAction(action ssa.Action) string {
	if c, ok := colorPerAction[action]; ok {
		return c.Sprint(action)
	}
	return action.String()
}

func colorizeChange(subject string, action ssa.Action) string {
	return fmt.Sprintf("%s %s", colorizeSubject(subject), colorizeAction(action))
}

func colorizeChangeSetEntry(change ssa.ChangeSetEntry) string {
	return colorizeChange(change.Subject, change.Action)
}

func colorizeDryRun(dryRun dryRunType) string {
	return colorDryRun.Sprint(string(dryRun))
}

func colorizeError(err error) string {
	return colorError.Sprint(err.Error())
}

func colorizeStatus(status status.Status) string {
	if c, ok := colorPerStatus[status]; ok {
		return c.Sprint(status)
	}
	return status.String()
}

func colorizeBundle(bundle string) string {
	return colorCallerPrefix.Sprint("b:") + colorBundle.Sprint(bundle)
}

func colorizeInstance(instance string) string {
	return colorCallerPrefix.Sprint("i:") + colorInstance.Sprint(instance)
}

func colorizeRuntime(runtime string) string {
	return colorCallerPrefix.Sprint("r:") + colorInstance.Sprint(runtime)
}

func colorizeCluster(cluster string) string {
	return colorCallerPrefix.Sprint("c:") + colorInstance.Sprint(cluster)
}

func LoggerBundle(ctx context.Context, bundle string) logr.Logger {
	if !rootArgs.prettyLog {
		return LoggerFrom(ctx, "bundle", bundle)
	}
	return LoggerFrom(ctx, "caller", colorizeBundle(bundle))
}

func LoggerInstance(ctx context.Context, instance string) logr.Logger {
	if !rootArgs.prettyLog {
		return LoggerFrom(ctx, "instance", instance)
	}
	return LoggerFrom(ctx, "caller", colorizeInstance(instance))
}

func LoggerBundleInstance(ctx context.Context, bundle, instance string) logr.Logger {
	if !rootArgs.prettyLog {
		return LoggerFrom(ctx, "bundle", bundle, "instance", instance)
	}
	return LoggerFrom(ctx, "caller", fmt.Sprintf("%s %s %s", colorizeBundle(bundle), color.CyanString(">"), colorizeInstance(instance)))
}

func LoggerRuntime(ctx context.Context, runtime, cluster string) logr.Logger {
	switch cluster {
	case apiv1.RuntimeDefaultName:
		if !rootArgs.prettyLog {
			return LoggerFrom(ctx, "runtime", runtime)
		}
		return LoggerFrom(ctx, "caller", colorizeRuntime(runtime))
	default:
		if !rootArgs.prettyLog {
			return LoggerFrom(ctx, "runtime", runtime, "cluster", cluster)
		}
		return LoggerFrom(ctx, "caller",
			fmt.Sprintf("%s %s %s", colorizeRuntime(runtime),
				color.CyanString(">"), colorizeCluster(cluster)))
	}
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
