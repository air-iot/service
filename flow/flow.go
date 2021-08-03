package flow

import "regexp"

var Reg, _ = regexp.Compile("\\${(.+?)}")
