package chat

import (
	"net/http"

	"gitee.com/matkurban/openimsdk-tools/apiresp"
	"gitee.com/matkurban/openimsdk-tools/log"
	"github.com/gin-gonic/gin"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/webhook"
)

// LiveKitWebhook receives LiveKit server webhooks (no chat token; LiveKit signature auth).
func (o *Api) LiveKitWebhook(c *gin.Context) {
	if o.MeetingManager == nil {
		apiresp.GinError(c, http.ErrNotSupported)
		return
	}

	authProvider := auth.NewSimpleKeyProvider(
		o.MeetingManager.LiveKitKey(),
		o.MeetingManager.LiveKitSecret(),
	)
	event, err := webhook.ReceiveWebhookEvent(c.Request, authProvider)
	if err != nil {
		log.ZWarn(c, "LiveKit webhook verify failed", err)
		c.Status(http.StatusUnauthorized)
		return
	}

	o.MeetingManager.EnqueueWebhookEvent(event)
	c.Status(http.StatusOK)
}
