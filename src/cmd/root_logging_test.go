package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestRootStartupDoesNotDumpViperSettings(t *testing.T) {
	const sentinel = "WEBHOOK-SECRET-SENTINEL"
	viper.Set("task5_sensitive_setting", sentinel)
	t.Cleanup(func() { viper.Set("task5_sensitive_setting", "") })

	output := captureStdoutForTest(t, initEnvConfig)
	if strings.Contains(output, sentinel) {
		t.Fatal("root startup exposed a Viper setting value")
	}
}

func captureStdoutForTest(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	previous := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previous })

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = previous
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(data)
}
