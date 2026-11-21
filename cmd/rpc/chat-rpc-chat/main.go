package main

import (
	"chat/pkg/common/cmd"
	"chat/pkg/common/program"
)

func main() {
	if err := cmd.NewChatRpcCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}
