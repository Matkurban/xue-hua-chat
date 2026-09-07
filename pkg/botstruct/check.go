package botstruct

import (
	"strings"

	"chat/pkg/common/constant"
)

func IsAgentUserID(userID string) bool {
	return strings.HasPrefix(userID, constant.AgentUserIDPrefix)
}
