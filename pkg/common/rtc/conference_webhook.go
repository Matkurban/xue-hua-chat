package rtc

import (
	"context"
	"time"

	"chat/pkg/common/log"
	"github.com/livekit/protocol/livekit"
)

func (m *ConferenceManager) EnqueueWebhookEvent(event *livekit.WebhookEvent) {
	select {
	case m.eventCh <- event:
	default:
		log.ZWarn(context.Background(), "conference webhook queue full", nil, "event", event.GetEvent())
	}
}

func (m *ConferenceManager) webhookWorker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stopCh:
			return
		case ev := <-m.eventCh:
			m.handleWebhook(context.Background(), ev)
		}
	}
}

func (m *ConferenceManager) handleWebhook(ctx context.Context, ev *livekit.WebhookEvent) {
	if ev == nil {
		return
	}
	ok, err := m.store.ClaimWebhookOnce(ctx, ev.GetId())
	if err != nil {
		log.ZWarn(ctx, "conference webhook claim failed", err, "event", ev.GetEvent(), "id", ev.GetId())
		// Fail open: still process so Redis blips do not drop room lifecycle.
	} else if !ok {
		return
	}
	switch ev.GetEvent() {
	case "participant_joined":
		if ev.Room != nil && ev.Participant != nil {
			m.MarkInLiveKit(ev.Room.Name, ev.Participant.Identity)
		}
	case "participant_left":
		if ev.Room == nil || ev.Participant == nil {
			return
		}
		roomID, userID := ev.Room.Name, ev.Participant.Identity
		mem, ok, _ := m.store.GetMember(ctx, roomID, userID)
		if !ok || mem == nil {
			// Graceful leave / kick already removed the member.
			return
		}
		// Abnormal disconnect: keep membership for a short reconnect grace.
		m.scheduleDisconnectGrace(ctx, roomID, userID, mem)
	case "room_finished":
		if ev.Room != nil {
			m.DeleteRoomFinished(ev.Room.Name)
		}
	case "egress_started", "egress_updated", "egress_ended":
		m.reconcileEgressWebhook(ctx, ev)
	case "ingress_started", "ingress_ended":
		m.reconcileIngressWebhook(ctx, ev)
	default:
	}
}

func (m *ConferenceManager) scheduleDisconnectGrace(ctx context.Context, roomID, userID string, mem *ConferenceMember) {
	mem.InLiveKit = false
	if err := m.store.SaveMember(ctx, roomID, mem); err != nil {
		log.ZWarn(ctx, "conference mark offline failed", err, "roomID", roomID, "userID", userID)
	}
	due := time.Now().Add(conferenceDisconnectGrace).UnixMilli()
	if err := m.store.SchedulePendingLeave(ctx, roomID, userID, due); err != nil {
		log.ZWarn(ctx, "conference schedule pending leave failed", err, "roomID", roomID, "userID", userID)
	}
}

func (m *ConferenceManager) applyPendingLeave(ref PendingLeaveRef) {
	ctx := context.Background()
	mem, ok, err := m.store.GetMember(ctx, ref.RoomID, ref.UserID)
	if err != nil || !ok || mem == nil {
		return
	}
	if mem.InLiveKit {
		return
	}
	if _, err := m.LeaveConference(ref.RoomID, ref.UserID); err != nil {
		log.ZWarn(ctx, "conference pending leave failed", err, "roomID", ref.RoomID, "userID", ref.UserID)
	}
}

func (m *ConferenceManager) pendingLeaveLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.drainPendingLeaves(context.Background())
		}
	}
}

func (m *ConferenceManager) drainPendingLeaves(ctx context.Context) {
	refs, err := m.store.ClaimDuePendingLeaves(ctx, time.Now().UnixMilli(), 64)
	if err != nil {
		log.ZWarn(ctx, "conference drain pending leaves failed", err)
		return
	}
	for _, ref := range refs {
		m.applyPendingLeave(ref)
	}
}

func isEgressTerminal(status livekit.EgressStatus) bool {
	switch status {
	case livekit.EgressStatus_EGRESS_COMPLETE,
		livekit.EgressStatus_EGRESS_FAILED,
		livekit.EgressStatus_EGRESS_ABORTED,
		livekit.EgressStatus_EGRESS_LIMIT_REACHED:
		return true
	default:
		return false
	}
}

func (m *ConferenceManager) reconcileEgressWebhook(ctx context.Context, ev *livekit.WebhookEvent) {
	info := ev.GetEgressInfo()
	if info == nil {
		return
	}
	roomID := info.GetRoomName()
	egressID := info.GetEgressId()
	if roomID == "" || egressID == "" {
		return
	}
	terminal := isEgressTerminal(info.GetStatus()) || ev.GetEvent() == "egress_ended"
	syncEvent := "egress_updated"
	switch {
	case terminal:
		syncEvent = "egress_ended"
	case ev.GetEvent() == "egress_started":
		syncEvent = "egress_started"
	}
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok || room == nil {
			return nil
		}
		if terminal {
			if room.EgressID != "" && room.EgressID != egressID {
				// A newer egress is active; ignore stale ended event.
				return nil
			}
			if room.EgressID == "" && syncEvent == "egress_ended" {
				return nil
			}
			room.EgressID = ""
			return m.bumpAndSync(lockCtx, room, syncEvent, "")
		}
		if room.EgressID == egressID && syncEvent == "egress_updated" {
			// No state change beyond status; still bump so clients refresh UI.
			return m.bumpAndSync(lockCtx, room, syncEvent, "")
		}
		room.EgressID = egressID
		return m.bumpAndSync(lockCtx, room, syncEvent, "")
	})
	if err != nil {
		log.ZWarn(ctx, "conference egress webhook reconcile failed", err,
			"roomID", roomID, "egressID", egressID, "event", ev.GetEvent())
	}
}

func isIngressTerminal(info *livekit.IngressInfo) bool {
	if info == nil {
		return true
	}
	st := info.GetState()
	if st == nil {
		return false
	}
	switch st.GetStatus() {
	case livekit.IngressState_ENDPOINT_ERROR, livekit.IngressState_ENDPOINT_COMPLETE:
		return true
	default:
		return false
	}
}

func (m *ConferenceManager) reconcileIngressWebhook(ctx context.Context, ev *livekit.WebhookEvent) {
	info := ev.GetIngressInfo()
	if info == nil {
		return
	}
	roomID := info.GetRoomName()
	ingressID := info.GetIngressId()
	if roomID == "" || ingressID == "" {
		return
	}
	terminal := isIngressTerminal(info) || ev.GetEvent() == "ingress_ended"
	syncEvent := "ingress_updated"
	if terminal {
		syncEvent = "ingress_ended"
	} else if ev.GetEvent() == "ingress_started" {
		syncEvent = "ingress_started"
	}
	err := m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok || room == nil {
			return nil
		}
		if terminal {
			if room.IngressID != "" && room.IngressID != ingressID {
				return nil
			}
			if room.IngressID == "" {
				return nil
			}
			room.IngressID = ""
			room.IngressURL = ""
			room.IngressStreamKey = ""
			return m.bumpAndSync(lockCtx, room, syncEvent, "")
		}
		room.IngressID = ingressID
		if u := info.GetUrl(); u != "" {
			room.IngressURL = u
		}
		if k := info.GetStreamKey(); k != "" {
			room.IngressStreamKey = k
		}
		return m.bumpAndSync(lockCtx, room, syncEvent, "")
	})
	if err != nil {
		log.ZWarn(ctx, "conference ingress webhook reconcile failed", err,
			"roomID", roomID, "ingressID", ingressID, "event", ev.GetEvent())
	}
}
