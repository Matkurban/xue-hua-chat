package chat

import (
	"net/http"

	"gitee.com/matkurban/openimsdk-tools/log"
	"github.com/gin-gonic/gin"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/webhook"
)

// ConferenceLiveKitWebhook receives LiveKit webhooks for conference rooms.
func (o *Api) ConferenceLiveKitWebhook(c *gin.Context) {
	if o.ConferenceManager == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	authProvider := auth.NewSimpleKeyProvider(
		o.ConferenceManager.LiveKitKey(),
		o.ConferenceManager.LiveKitSecret(),
	)
	event, err := webhook.ReceiveWebhookEvent(c.Request, authProvider)
	if err != nil {
		log.ZWarn(c, "conference LiveKit webhook verify failed", err)
		c.Status(http.StatusUnauthorized)
		return
	}
	o.ConferenceManager.EnqueueWebhookEvent(event)
	c.Status(http.StatusOK)
}
