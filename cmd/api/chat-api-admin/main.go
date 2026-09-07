package main

import (
	"chat/pkg/common/cmd"
	"gitee.com/matkurban/openimsdk-tools/system/program"
)

func main() {
	if err := cmd.NewAdminApiCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}
