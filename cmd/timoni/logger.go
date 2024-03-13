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

package main

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/go-logr/logr"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/logger"
)

func loggerBundle(ctx context.Context, bundle, cluster string, prettify bool) logr.Logger {
	switch cluster {
	case apiv1.RuntimeDefaultName:
		if !prettify {
			return LoggerFrom(ctx, "bundle", bundle)
		}
		return LoggerFrom(ctx, "caller", logger.ColorizeBundle(bundle))
	default:
		if !prettify {
			return LoggerFrom(ctx, "bundle", bundle, "cluster", cluster)
		}
		return LoggerFrom(ctx, "caller",
			fmt.Sprintf("%s %s %s",
				logger.ColorizeBundle(bundle),
				color.CyanString(">"),
				logger.ColorizeCluster(cluster)))
	}
}

func loggerInstance(ctx context.Context, instance string, prettify bool) logr.Logger {
	if !prettify {
		return LoggerFrom(ctx, "instance", instance)
	}
	return LoggerFrom(ctx, "caller", logger.ColorizeInstance(instance))
}

func loggerBundleInstance(ctx context.Context, bundle, cluster, instance string, prettify bool) logr.Logger {
	switch cluster {
	case apiv1.RuntimeDefaultName:
		if !prettify {
			return LoggerFrom(ctx, "bundle", bundle, "instance", instance)
		}
		return LoggerFrom(ctx, "caller",
			fmt.Sprintf("%s %s %s",
				logger.ColorizeBundle(bundle),
				color.CyanString(">"),
				logger.ColorizeInstance(instance)))
	default:
		if !prettify {
			return LoggerFrom(ctx, "bundle", bundle, "cluster", cluster, "instance", instance)
		}
		return LoggerFrom(ctx, "caller",
			fmt.Sprintf("%s %s %s %s %s",
				logger.ColorizeBundle(bundle),
				color.CyanString(">"),
				logger.ColorizeCluster(cluster),
				color.CyanString(">"),
				logger.ColorizeInstance(instance)))

	}
}

func loggerRuntime(ctx context.Context, runtime, cluster string, prettify bool) logr.Logger {
	switch cluster {
	case apiv1.RuntimeDefaultName:
		if !prettify {
			return LoggerFrom(ctx, "runtime", runtime)
		}
		return LoggerFrom(ctx, "caller", logger.ColorizeRuntime(runtime))
	default:
		if !prettify {
			return LoggerFrom(ctx, "runtime", runtime, "cluster", cluster)
		}
		return LoggerFrom(ctx, "caller",
			fmt.Sprintf("%s %s %s", logger.ColorizeRuntime(runtime),
				color.CyanString(">"), logger.ColorizeCluster(cluster)))
	}
}

// LoggerFrom returns a logr.Logger with predefined values from a context.Context.
func LoggerFrom(ctx context.Context, keysAndValues ...interface{}) logr.Logger {
	if cliLogger.IsZero() {
		cliLogger = logger.NewConsoleLogger(false, false)
	}
	newLogger := cliLogger
	if ctx != nil {
		if l, err := logr.FromContext(ctx); err == nil {
			newLogger = l
		}
	}
	return newLogger.WithValues(keysAndValues...)
}
