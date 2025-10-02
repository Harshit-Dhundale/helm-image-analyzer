package chart

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func RenderWithHelm(ctx context.Context, chartDir, valuesYAML string, set map[string]string) (rendered string, docCount int, err error) {
	helmBin := "helm"
	if _, err = exec.LookPath(helmBin); err != nil {
		return "", 0, errors.Wrap(err, "helm not found in PATH")
	}

	var valsFile string
	cleanup := func() {}
	if valuesYAML != "" {
		tmp, err := os.MkdirTemp("", "values-*")
		if err != nil {
			return "", 0, errors.Wrap(err, "mktemp values")
		}
		cleanup = func() { _ = os.RemoveAll(tmp) }
		valsFile = filepath.Join(tmp, "values.yaml")
		if err := os.WriteFile(valsFile, []byte(valuesYAML), 0o644); err != nil {
			cleanup()
			return "", 0, errors.Wrap(err, "write values.yaml")
		}
	}

	args := []string{"template", "chart-analyzer", chartDir}
	if valsFile != "" {
		args = append(args, "-f", valsFile)
	}
	for k, v := range set {
		args = append(args, "--set", k+"="+v)
	}

	cmd := exec.CommandContext(ctx, helmBin, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		cleanup()
		return "", 0, errors.Wrapf(err, "helm template error: %s", stderr.String())
	}
	cleanup()


	renderedStr := strings.TrimSpace(out.String())
	if renderedStr == "" {
		return "", 0, nil
	}
	docs := 1
	for _, line := range strings.Split(renderedStr, "\n") {
		if strings.TrimSpace(line) == "---" {
			docs++
		}
	}
	return renderedStr, docs, nil
}
