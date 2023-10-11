package main

import (
	"fmt"
	"runtime"

	"github.com/jnsgruk/gosherve/cmd"
)

var (
	version string = "dev"
	commit  string
	date    string
)

func main() {
	versionString := buildVersion(version, commit, date)
	cmd.Execute(versionString)
}

func buildVersion(version, commit, date string) string {
	result := version
	if commit != "" {
		result = fmt.Sprintf("%s\ncommit: %s", result, commit)
	}
	if date != "" {
		result = fmt.Sprintf("%s\nbuilt at: %s", result, date)
	}
	result = fmt.Sprintf("%s\ngoos: %s\ngoarch: %s", result, runtime.GOOS, runtime.GOARCH)
	return result
}
