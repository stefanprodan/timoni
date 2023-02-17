package main

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/mattn/go-shellwords"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func executeCommand(cmd string) (string, error) {
	defer resetCmdArgs()
	args, err := shellwords.Parse(cmd)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)

	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	logger.stderr = rootCmd.ErrOrStderr()

	_, err = rootCmd.ExecuteC()
	result := buf.String()

	return result, err
}

func resetCmdArgs() {
	applyArgs = applyFlags{}
	buildArgs = buildFlags{}
	deleteArgs = deleteFlags{}
	inspectModuleArgs = inspectModuleFlags{}
	inspectResourcesArgs = inspectResourcesFlags{}
	inspectValuesArgs = inspectValuesFlags{}
	lintArgs = lintFlags{}
	listArgs = listFlags{}
	pullArgs = pullFlags{}
	pushArgs = pushFlags{}
}

func rnd(prefix string, n int) string {
	runes := []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))]
	}
	return prefix + "-" + string(b)
}
