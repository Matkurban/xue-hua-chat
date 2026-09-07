package rtc

import (
	"context"
	"encoding/json"
	"fmt"
)

func normalizeStageLimit(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func removeUserID(ids []string, userID string) []string {
	if len(ids) == 0 || userID == "" {
		return ids
	}
	out := ids[:0]
	for _, id := range ids {
		if id != userID {
			out = append(out, id)
		}
	}
	return out
}

func containsUserID(ids []string, userID string) bool {
	for _, id := range ids {
		if id == userID {
			return true
		}
	}
	return false
}

func (m *ConferenceManager) countOnStage(ctx context.Context, roomID string) (int, error) {
	ids, err := m.store.AllMemberIDs(ctx, roomID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		mem, ok, err := m.store.GetMember(ctx, roomID, id)
		if err != nil {
			return 0, err
		}
		if ok && mem.OnStage {
			n++
		}
	}
	return n, nil
}

// CountOnStage returns how many members are currently on stage.
func (m *ConferenceManager) CountOnStage(roomID string) int {
	n, err := m.countOnStage(context.Background(), roomID)
	if err != nil {
		return 0
	}
	return n
}

func (m *ConferenceManager) collectPendingApply(ctx context.Context, roomID string) ([]string, error) {
	ids, err := m.store.AllMemberIDs(ctx, roomID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, id := range ids {
		mem, ok, err := m.store.GetMember(ctx, roomID, id)
		if err != nil {
			return nil, err
		}
		if ok && mem.SpeakPending {
			out = append(out, id)
		}
	}
	return out, nil
}

func (m *ConferenceManager) clearMemberStageOnLeave(ctx context.Context, room *ConferenceRoom, userID string) {
	if room == nil || userID == "" {
		return
	}
	room.CameraOnUserIDs = removeUserID(room.CameraOnUserIDs, userID)
}

func (m *ConferenceManager) buildStageState(ctx context.Context, room *ConferenceRoom, target *ConferenceMember) ConferenceStageState {
	st := ConferenceStageState{
		CameraOnUserIDs: append([]string(nil), room.CameraOnUserIDs...),
		CameraOnCount:   len(room.CameraOnUserIDs),
		AutoStageLimit:  room.AutoStageLimit,
		LinkMicEnabled:  roomLinkMicEnabled(room),
	}
	if n, err := m.countOnStage(ctx, room.RoomID); err == nil {
		st.OnStageCount = n
	}
	if pending, err := m.collectPendingApply(ctx, room.RoomID); err == nil {
		st.PendingApplyUserIDs = pending
	}
	if target != nil {
		st.TargetOnStage = boolPtr(target.OnStage)
		st.TargetSpeakPending = boolPtr(target.SpeakPending)
		st.TargetCameraOn = boolPtr(target.CameraOn)
	}
	return st
}

func (m *ConferenceManager) attachStageState(ctx context.Context, room *ConferenceRoom, payload *ConferenceSyncPayload, target *ConferenceMember) {
	if payload == nil || room == nil {
		return
	}
	st := m.buildStageState(ctx, room, target)
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	payload.StageStateJSON = string(b)
}

func cameraBlockedByGlobalPolicy(room *ConferenceRoom, mem *ConferenceMember) bool {
	return (room.GlobalVideoOff && !memberIsModerator(room, mem)) || mem.VideoForcedOff
}

// CanOpenCamera reports whether mem may turn the camera on (on-stage + policy).
func CanOpenCamera(room *ConferenceRoom, mem *ConferenceMember) bool {
	if room == nil || mem == nil || !mem.OnStage {
		return false
	}
	if cameraBlockedByGlobalPolicy(room, mem) {
		return false
	}
	return true
}

// ensureRoomStageDefaults migrates legacy rooms (limits unset) once.
func (m *ConferenceManager) ensureRoomStageDefaults(ctx context.Context, room *ConferenceRoom) error {
	if room == nil {
		return nil
	}
	if room.AutoStageLimit > 0 {
		return nil
	}
	room.AutoStageLimit = normalizeStageLimit(room.AutoStageLimit, m.autoStageLimit)
	ids, err := m.store.AllMemberIDs(ctx, room.RoomID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		mem, ok, err := m.store.GetMember(ctx, room.RoomID, id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if !mem.OnStage {
			mem.OnStage = true
			if err := m.store.SaveMember(ctx, room.RoomID, mem); err != nil {
				return err
			}
		}
	}
	return m.store.SaveRoom(ctx, room)
}

// ApplySpeak queues an audience member for host approval.
func (m *ConferenceManager) ApplySpeak(roomID, userID string) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("conference not found")
		}
		if err := m.ensureRoomStageDefaults(lockCtx, room); err != nil {
			return err
		}
		mem, ok, err := m.store.GetMember(lockCtx, roomID, userID)
		if err != nil || !ok {
			return fmt.Errorf("member not found")
		}
		if mem.OnStage {
			return nil // already on stage
		}
		if room.LinkMicDisabled {
			return fmt.Errorf("link_mic_disabled")
		}
		if mem.SpeakPending {
			return nil // idempotent
		}
		onStageCount, err := m.countOnStage(lockCtx, roomID)
		if err != nil {
			return err
		}
		if onStageCount >= room.AutoStageLimit {
			return fmt.Errorf("stage_slot_full")
		}
		mem.SpeakPending = true
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		payload := BuildConferenceSync(room, "speak_apply", userID)
		m.attachStageState(lockCtx, room, payload, mem)
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

// ApproveSpeak accepts or rejects a speak application (admin / co-host).
func (m *ConferenceManager) ApproveSpeak(roomID, hostID, targetUserID string, accept bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, hostID)
		if err != nil {
			return err
		}
		if err := m.ensureRoomStageDefaults(lockCtx, room); err != nil {
			return err
		}
		mem, ok, err := m.store.GetMember(lockCtx, roomID, targetUserID)
		if err != nil || !ok {
			return fmt.Errorf("member not found")
		}
		event := "speak_rejected"
		if accept {
			if room.LinkMicDisabled {
				return fmt.Errorf("link_mic_disabled")
			}
			if !mem.OnStage {
				onStageCount, err := m.countOnStage(lockCtx, roomID)
				if err != nil {
					return err
				}
				if onStageCount >= room.AutoStageLimit {
					return fmt.Errorf("stage_slot_full")
				}
			}
			mem.OnStage = true
			mem.SpeakPending = false
			event = "speak_approved"
		} else {
			mem.SpeakPending = false
		}
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		payload := BuildConferenceSync(room, event, targetUserID)
		m.attachStageState(lockCtx, room, payload, mem)
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

// LeaveSpeak moves a speaker back to audience. Empty targetUserID means the operator
// leaves themselves. A different target requires host/co-host and cannot be the room host.
func (m *ConferenceManager) LeaveSpeak(roomID, operatorID, targetUserID string) error {
	if targetUserID == "" {
		targetUserID = operatorID
	}
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("conference not found")
		}
		if err := m.ensureRoomStageDefaults(lockCtx, room); err != nil {
			return err
		}
		if targetUserID != operatorID {
			if _, err := m.requireModerator(lockCtx, roomID, operatorID); err != nil {
				return err
			}
		}
		if targetUserID == room.AdminUserID {
			return fmt.Errorf("admin cannot leave stage")
		}
		mem, ok, err := m.store.GetMember(lockCtx, roomID, targetUserID)
		if err != nil || !ok {
			return fmt.Errorf("member not found")
		}
		if !mem.OnStage && !mem.SpeakPending {
			return nil
		}
		mem.OnStage = false
		mem.SpeakPending = false
		mem.CameraOn = false
		room.CameraOnUserIDs = removeUserID(room.CameraOnUserIDs, targetUserID)
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		if err := m.store.SaveRoom(lockCtx, room); err != nil {
			return err
		}
		m.forceMuteMediaKind(lockCtx, roomID, targetUserID, true)
		m.forceMuteMediaKind(lockCtx, roomID, targetUserID, false)
		m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		payload := BuildConferenceSync(room, "speak_left", targetUserID)
		m.attachStageState(lockCtx, room, payload, mem)
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

// SetLinkMic turns audience raise-hand / approve on or off (host or co-host).
// Disabling clears pending applies; people already on stage stay.
func (m *ConferenceManager) SetLinkMic(roomID, adminID string, enabled bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, err := m.requireModerator(lockCtx, roomID, adminID)
		if err != nil {
			return err
		}
		if err := m.ensureRoomStageDefaults(lockCtx, room); err != nil {
			return err
		}
		disabled := !enabled
		if room.LinkMicDisabled == disabled {
			payload := BuildConferenceSync(room, "link_mic", "")
			m.attachStageState(lockCtx, room, payload, nil)
			return m.bumpAndSyncPayload(lockCtx, room, payload)
		}
		room.LinkMicDisabled = disabled
		if disabled {
			if err := m.clearPendingApplies(lockCtx, roomID); err != nil {
				return err
			}
		}
		if err := m.store.SaveRoom(lockCtx, room); err != nil {
			return err
		}
		payload := BuildConferenceSync(room, "link_mic", "")
		m.attachStageState(lockCtx, room, payload, nil)
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}

func (m *ConferenceManager) clearPendingApplies(ctx context.Context, roomID string) error {
	ids, err := m.store.AllMemberIDs(ctx, roomID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		mem, ok, err := m.store.GetMember(ctx, roomID, id)
		if err != nil {
			return err
		}
		if !ok || !mem.SpeakPending {
			continue
		}
		mem.SpeakPending = false
		if err := m.store.SaveMember(ctx, roomID, mem); err != nil {
			return err
		}
	}
	return nil
}

// ToggleCamera updates authoritative camera-on list (on-stage + policy only).
func (m *ConferenceManager) ToggleCamera(roomID, userID string, enabled bool) error {
	ctx := context.Background()
	return m.store.WithRoomLock(ctx, roomID, func(lockCtx context.Context) error {
		room, ok, err := m.store.GetRoom(lockCtx, roomID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("conference not found")
		}
		if err := m.ensureRoomStageDefaults(lockCtx, room); err != nil {
			return err
		}
		mem, ok, err := m.store.GetMember(lockCtx, roomID, userID)
		if err != nil || !ok {
			return fmt.Errorf("member not found")
		}
		if enabled {
			if !mem.OnStage {
				return fmt.Errorf("audience cannot open camera")
			}
			if cameraBlockedByGlobalPolicy(room, mem) {
				return fmt.Errorf("camera disabled by policy")
			}
			mem.CameraOn = true
			if !containsUserID(room.CameraOnUserIDs, userID) {
				room.CameraOnUserIDs = append(room.CameraOnUserIDs, userID)
			}
		} else {
			mem.CameraOn = false
			room.CameraOnUserIDs = removeUserID(room.CameraOnUserIDs, userID)
			m.forceMuteMediaKind(lockCtx, roomID, userID, true)
		}
		if err := m.store.SaveMember(lockCtx, roomID, mem); err != nil {
			return err
		}
		if err := m.store.SaveRoom(lockCtx, room); err != nil {
			return err
		}
		m.syncParticipantPublishPerm(lockCtx, room, mem, memberCanPublishData(room, mem))
		payload := BuildConferenceSync(room, "camera_toggled", userID)
		m.attachStageState(lockCtx, room, payload, mem)
		return m.bumpAndSyncPayload(lockCtx, room, payload)
	})
}
