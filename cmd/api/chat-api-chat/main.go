package main

import (
	"chat/pkg/common/cmd"
	"chat/pkg/common/program"
)

func main() {
	if err := cmd.NewChatApiCmd().Exec(); err != nil {
		program.ExitWithError(err)
	}
}
