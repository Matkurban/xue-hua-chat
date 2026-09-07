package rtc

import (
	"context"
	"strings"

	"chat/pkg/common/mctx"
)

// ListParticipants pages conference members.
// Unfiltered lists use Redis ZSET range. Keyword / role filter scan then page.
func (m *ConferenceManager) ListParticipants(roomID string, pageNumber, pageSize int32, roleFilter, keyword string) (*ConferenceParticipantPage, error) {
	ctx := context.Background()
	if pageNumber < 1 {
		pageNumber = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}
	kw := strings.TrimSpace(keyword)

	// Fast path: no keyword and no role filter → ZSET offset/limit.
	if kw == "" && roleFilter == "" {
		total, err := m.store.MemberCount(ctx, roomID)
		if err != nil {
			return nil, err
		}
		offset := int64((pageNumber - 1) * pageSize)
		ids, err := m.store.ListMemberIDs(ctx, roomID, offset, int64(pageSize))
		if err != nil {
			return nil, err
		}
		list := m.loadMembers(ctx, roomID, ids, "")
		m.enrichNicknames(ctx, roomID, list)
		return &ConferenceParticipantPage{
			Total: total, PageNumber: pageNumber, PageSize: pageSize, List: list,
		}, nil
	}

	allIDs, err := m.store.AllMemberIDs(ctx, roomID)
	if err != nil {
		return nil, err
	}
	filtered := m.loadMembers(ctx, roomID, allIDs, roleFilter)
	m.enrichNicknames(ctx, roomID, filtered)
	if kw != "" {
		low := strings.ToLower(kw)
		out := filtered[:0]
		for _, mem := range filtered {
			if strings.Contains(strings.ToLower(mem.UserID), low) ||
				strings.Contains(strings.ToLower(mem.Nickname), low) {
				out = append(out, mem)
			}
		}
		filtered = out
	}
	total := int64(len(filtered))
	start := int((pageNumber - 1) * pageSize)
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + int(pageSize)
	if end > len(filtered) {
		end = len(filtered)
	}
	return &ConferenceParticipantPage{
		Total: total, PageNumber: pageNumber, PageSize: pageSize, List: filtered[start:end],
	}, nil
}

func (m *ConferenceManager) loadMembers(ctx context.Context, roomID string, ids []string, roleFilter string) []*ConferenceMember {
	list := make([]*ConferenceMember, 0, len(ids))
	for _, id := range ids {
		mem, ok, _ := m.store.GetMember(ctx, roomID, id)
		if !ok {
			continue
		}
		if roleFilter != "" && mem.Role != roleFilter {
			continue
		}
		list = append(list, mem)
	}
	return list
}

// fillMemberProfile loads Nickname/FaceURL from IM for a single member (create/join).
func (m *ConferenceManager) fillMemberProfile(ctx context.Context, mem *ConferenceMember) {
	if m == nil || m.im == nil || mem == nil || mem.UserID == "" {
		return
	}
	if mem.Nickname != "" && mem.FaceURL != "" {
		return
	}
	tok, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		return
	}
	u, err := m.im.GetUserInfo(mctx.WithApiToken(ctx, tok), mem.UserID)
	if err != nil || u == nil {
		return
	}
	if mem.Nickname == "" {
		mem.Nickname = u.Nickname
	}
	if mem.FaceURL == "" {
		mem.FaceURL = u.FaceURL
	}
}

// enrichNicknames fills empty Nickname/FaceURL from IM and persists back to Redis
// so keyword search can match display names (runs before filter).
func (m *ConferenceManager) enrichNicknames(ctx context.Context, roomID string, list []*ConferenceMember) {
	if m.im == nil || len(list) == 0 {
		return
	}
	need := make([]string, 0)
	for _, mem := range list {
		if mem.Nickname == "" || mem.FaceURL == "" {
			need = append(need, mem.UserID)
		}
	}
	if len(need) == 0 {
		return
	}
	tok, err := m.im.ImAdminTokenWithDefaultAdmin(ctx)
	if err != nil {
		return
	}
	users, err := m.im.GetUsersInfo(mctx.WithApiToken(ctx, tok), need)
	if err != nil {
		return
	}
	byID := make(map[string]*struct{ nick, face string }, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		byID[u.UserID] = &struct{ nick, face string }{u.Nickname, u.FaceURL}
	}
	for _, mem := range list {
		p, ok := byID[mem.UserID]
		if !ok {
			continue
		}
		changed := false
		if mem.Nickname == "" && p.nick != "" {
			mem.Nickname = p.nick
			changed = true
		}
		if mem.FaceURL == "" && p.face != "" {
			mem.FaceURL = p.face
			changed = true
		}
		if changed {
			_ = m.store.SaveMember(ctx, roomID, mem)
		}
	}
}
