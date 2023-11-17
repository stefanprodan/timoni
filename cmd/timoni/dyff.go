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
	"path/filepath"
	"sort"

	"github.com/fluxcd/pkg/ssa"
	"github.com/gonvenience/ytbx"
	"github.com/homeport/dyff/pkg/dyff"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
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

func instanceDryRunDiff(ctx context.Context,
	rm *ssa.ResourceManager,
	objects []*unstructured.Unstructured,
	staleObjects []*unstructured.Unstructured,
	nsExists bool,
	tmpDir string,
	withDiff bool) error {
	log := LoggerFrom(ctx)
	diffOpts := ssa.DefaultDiffOptions()
	sort.Sort(ssa.SortableUnstructureds(objects))

	for _, r := range objects {
		if !nsExists {
			log.Info(colorizeJoin(r, ssa.CreatedAction, dryRunServer))
			continue
		}

		change, liveObject, mergedObject, err := rm.Diff(ctx, r, diffOpts)
		if err != nil {
			if ssa.IsImmutableError(err) {
				if ssa.AnyInMetadata(r, map[string]string{
					apiv1.ForceAction: apiv1.EnabledValue,
				}) {
					log.Info(colorizeJoin(r, ssa.CreatedAction, dryRunServer))
				} else {
					log.Error(nil, colorizeJoin(r, "immutable", dryRunServer))
				}
			} else {
				log.Error(err, colorizeUnstructured(r))
			}

			continue
		}

		log.Info(colorizeJoin(change, dryRunServer))
		if withDiff && change.Action == ssa.ConfiguredAction {
			liveYAML, _ := yaml.Marshal(liveObject)
			liveFile := filepath.Join(tmpDir, "live.yaml")
			if err := os.WriteFile(liveFile, liveYAML, 0644); err != nil {
				return err
			}

			mergedYAML, _ := yaml.Marshal(mergedObject)
			mergedFile := filepath.Join(tmpDir, "merged.yaml")
			if err := os.WriteFile(mergedFile, mergedYAML, 0644); err != nil {
				return err
			}

			if err := diffYAML(liveFile, mergedFile, rootCmd.OutOrStdout()); err != nil {
				return err
			}
		}
	}

	for _, r := range staleObjects {
		log.Info(colorizeJoin(r, ssa.DeletedAction, dryRunServer))
	}

	return nil
}
