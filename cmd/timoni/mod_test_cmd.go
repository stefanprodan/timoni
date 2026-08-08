/*
Copyright 2026 Stefan Prodan

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
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
)

var testModCmd = &cobra.Command{
	Use:   "test [MODULE PATH]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Run the test cases of a local module",
	Long: `The test command runs the test cases declared in the module's '*_test.cue' files.
Each case is built in isolation with its own values, and its expectations are
checked against the resulting Kubernetes objects.`,
	Example: `  # run all test cases of a module in the current directory
  timoni mod test

  # run the test cases matching a regular expression
  timoni mod test ./path/to/module --run '^service'
`,
	RunE: runTestModCmd,
}

type testModFlags struct {
	path string
	pkg  flags.Package
	run  string
}

var testModArgs testModFlags

func init() {
	testModCmd.Flags().VarP(&testModArgs.pkg, testModArgs.pkg.Type(), testModArgs.pkg.Shorthand(), testModArgs.pkg.Description())
	testModCmd.Flags().StringVar(&testModArgs.run, "run", "",
		"Regular expression matching the names of the test cases to run.")
	modCmd.AddCommand(testModCmd)
}

func runTestModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		testModArgs.path = "."
	} else {
		testModArgs.path = args[0]
	}

	if fs, err := os.Stat(testModArgs.path); err != nil || !fs.IsDir() {
		return fmt.Errorf("module not found at path %s", testModArgs.path)
	}

	// The CUE loader requires absolute paths for the in-memory overlays
	// the module builder passes it.
	moduleRoot, err := filepath.Abs(testModArgs.path)
	if err != nil {
		return err
	}

	var filter *regexp.Regexp
	if testModArgs.run != "" {
		filter, err = regexp.Compile(testModArgs.run)
		if err != nil {
			return fmt.Errorf("invalid --run expression: %w", err)
		}
	}

	tester := engine.NewModuleTester(cuecontext.New(), moduleRoot, testModArgs.pkg.String())

	cases, err := tester.LoadCases()
	if err != nil {
		return describeErr(moduleRoot, "loading tests failed", err)
	}

	out := cmd.OutOrStdout()
	var run, failed int

	for _, tc := range cases {
		if filter != nil && !filter.MatchString(tc.Name) {
			continue
		}

		run++
		result := tester.Run(tc)
		if result.Passed() {
			fmt.Fprintf(out, "%s %s\n", logger.ColorizeInfo("PASS"), tc.Name)
			continue
		}

		failed++
		fmt.Fprintf(out, "%s %s\n", logger.ColorizeFailure("FAIL"), tc.Name)
		fmt.Fprintln(out, describeTestErr(moduleRoot, result.Err))
	}

	if len(cases) == 0 {
		fmt.Fprintln(out, "no test cases found")
		return nil
	}

	// A filter that matches nothing is a typo more often than an empty
	// selection, and passing on it would turn a mistyped run into a green one.
	if run == 0 {
		return fmt.Errorf("no test cases match the --run expression %q", testModArgs.run)
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d test cases failed", failed, run)
	}

	fmt.Fprintf(out, "%s %d test cases passed\n", logger.ColorizeInfo("OK"), run)
	return nil
}

// describeTestErr renders a test case failure indented under the case name.
// CUE errors are expanded so that every field that did not hold is reported
// with its position in the test file, rather than only the first.
func describeTestErr(moduleRoot string, err error) string {
	details := errors.Details(err, &errors.Config{Cwd: moduleRoot})

	lines := strings.Split(strings.TrimRight(details, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "     " + line
	}

	return logger.ColorizeFailure(strings.Join(lines, "\n"))
}
