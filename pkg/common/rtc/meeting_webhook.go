package rtc

import (
	"context"

	"chat/pkg/common/log"
	"github.com/livekit/protocol/livekit"
)

// EnqueueWebhookEvent queues a LiveKit webhook for async processing.
func (m *MeetingManager) EnqueueWebhookEvent(event *livekit.WebhookEvent) {
	select {
	case m.eventCh <- event:
	default:
		log.ZWarn(context.Background(), "meeting webhook event channel full, dropping event", nil,
			"event", event.GetEvent(), "room", event.GetRoom().GetName())
	}
}

func (m *MeetingManager) webhookWorker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stopCh:
			return
		case event := <-m.eventCh:
			if event == nil {
				continue
			}
			m.handleWebhookEvent(context.Background(), event)
		}
	}
}

func (m *MeetingManager) handleWebhookEvent(ctx context.Context, event *livekit.WebhookEvent) {
	room := event.GetRoom()
	if room == nil {
		return
	}
	roomID := room.GetName()
	if roomID == "" {
		return
	}

	switch event.GetEvent() {
	case "participant_joined":
		participant := event.GetParticipant()
		if participant == nil {
			return
		}
		userID := participant.GetIdentity()
		if userID == "" {
			return
		}
		m.MarkParticipantInLiveKit(roomID, userID)

	case "participant_left":
		participant := event.GetParticipant()
		if participant == nil {
			return
		}
		userID := participant.GetIdentity()
		if userID == "" {
			return
		}
		reason := participant.GetDisconnectReason().String()
		if !m.IsParticipantActive(roomID, userID) {
			log.ZDebug(ctx, "participant_left ignored (graceful hangup)", "roomID", roomID, "userID", userID, "reason", reason)
			return
		}
		if err := m.HandleAbnormalLeave(ctx, roomID, userID, reason); err != nil {
			log.ZWarn(ctx, "HandleAbnormalLeave failed", err, "roomID", roomID, "userID", userID)
		}

	case "room_finished":
		m.DeleteRoomFinished(roomID)
	}
}
