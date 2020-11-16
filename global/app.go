package global

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ROOT       string
	ConfigRoot string
)

const mainIniPath = "/etc/"

func init() {
	curFilename := os.Args[0]
	binaryPath, err := exec.LookPath(curFilename)
	if err != nil {
		panic(err)
	}
	binaryPath, err = filepath.Abs(binaryPath)
	if err != nil {
		panic(err)
	}
	ROOT = filepath.Dir(filepath.Dir(binaryPath))
	configPath := ROOT + mainIniPath
	if !FileExist(configPath) {
		curDir, _ := os.Getwd()
		pos := strings.LastIndex(curDir, "")
		if pos == -1 {
			panic("can't find " + mainIniPath)
		}
		ROOT = curDir[:pos]
		configPath = ROOT + mainIniPath
	}
	ConfigRoot = ROOT + "/etc/"
}

// fileExist 检查文件或目录是否存在
// 如果由 filename 指定的文件或目录存在则返回 true，否则返回 false
func FileExist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}
