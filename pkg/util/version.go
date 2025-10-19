package util

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const AppName = "webplus-openai"

var (
	version      = readVersionFile(".version") // value from VERSION file
	buildDate    = "1970-01-01T00:00:00Z"      // output from `date -u +'%Y-%m-%dT%H:%M:%SZ'`
	gitCommit    = "internal"                  // output from `git rev-parse HEAD`
	gitTag       = ""                          // output from `git describe --exact-match --tags HEAD` (if clean tree state)
	gitTreeState = ""                          // determined from `git status --porcelain`. either 'clean' or 'dirty'
)

type Version struct {
	AppName      string
	Version      string `json:"version" protobuf:"bytes,1,opt,name=version"`
	BuildDate    string `json:"buildDate" protobuf:"bytes,2,opt,name=buildDate"`
	GitCommit    string `json:"gitCommit" protobuf:"bytes,3,opt,name=gitCommit"`
	GitTag       string `json:"gitTag" protobuf:"bytes,4,opt,name=gitTag"`
	GitTreeState string `json:"gitTreeState" protobuf:"bytes,5,opt,name=gitTreeState"`
	GoVersion    string `json:"goVersion" protobuf:"bytes,6,opt,name=goVersion"`
	Compiler     string `json:"compiler" protobuf:"bytes,7,opt,name=compiler"`
	Platform     string `json:"platform" protobuf:"bytes,8,opt,name=platform"`
}

func GetVersion() Version {

	return Version{
		AppName:      AppName,
		Version:      version,
		BuildDate:    buildDate,
		GitCommit:    gitCommit,
		GitTag:       gitTag,
		GitTreeState: gitTreeState,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

func readVersionFile(filename string) string {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "dev" // 默认值
	}
	return strings.TrimSpace(string(data))
}
