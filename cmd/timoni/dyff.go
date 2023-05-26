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
	"fmt"
	"io"

	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
)

// DyffPrinter is a printer that prints dyff reports.
type DyffPrinter struct {
	OmitHeader bool
}

// NewDyffPrinter returns a new DyffPrinter.
func NewDyffPrinter() *DyffPrinter {
	return &DyffPrinter{
		OmitHeader: true,
	}
}

// Print prints the given args to the given writer.
func (p *DyffPrinter) Print(w io.Writer, args ...interface{}) error {
	for _, arg := range args {
		switch arg := arg.(type) {
		case dyff.Report:
			reportWriter := &dyff.HumanReport{
				Report:     arg,
				OmitHeader: p.OmitHeader,
			}

			if err := reportWriter.WriteReport(w); err != nil {
				return fmt.Errorf("failed to print report: %w", err)
			}
		default:
			return fmt.Errorf("unsupported type %T", arg)
		}
	}
	return nil
}

func diffYAML(liveFile, mergedFile string, output io.Writer) error {
	from, to, err := ytbx.LoadFiles(liveFile, mergedFile)
	if err != nil {
		return fmt.Errorf("failed to load input files: %w", err)
	}

	report, err := dyff.CompareInputFiles(from, to,
		dyff.IgnoreOrderChanges(false),
		dyff.KubernetesEntityDetection(true),
	)
	if err != nil {
		return fmt.Errorf("failed to compare input files: %w", err)
	}

	printer := NewDyffPrinter()
	return printer.Print(output, report)
}
