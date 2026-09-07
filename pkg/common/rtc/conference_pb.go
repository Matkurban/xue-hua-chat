package rtc

import (
	"errors"
	"math"
)

// Manual protobuf wire codec for ConferenceSync (matches conference.proto field numbers).
// Avoids requiring protoc in CI while still producing real proto3 binary payloads.

func MarshalConferenceSyncProto(p *ConferenceSyncPayload) []byte {
	if p == nil {
		return nil
	}
	b := make([]byte, 0, 128)
	b = appendProtoString(b, 1, p.RoomID)
	b = appendProtoVarint(b, 2, uint64(p.Rev))
	b = appendProtoString(b, 3, p.AdminUserID)
	b = appendProtoVarint(b, 4, uint64(uint32(p.ParticipantCount)))
	b = appendProtoBool(b, 5, p.GlobalAudioMuted)
	b = appendProtoBool(b, 6, p.GlobalVideoOff)
	b = appendProtoBool(b, 7, p.ChatMuted)
	b = appendProtoBool(b, 8, p.Locked)
	b = appendProtoString(b, 9, p.PinnedUserID)
	b = appendProtoString(b, 10, p.EgressID)
	b = appendProtoString(b, 11, p.IngressID)
	b = appendProtoString(b, 12, p.Event)
	b = appendProtoString(b, 13, p.TargetUserID)
	b = appendProtoBool(b, 14, p.TargetAudioForcedOff)
	b = appendProtoBool(b, 15, p.TargetVideoForcedOff)
	b = appendProtoVarint(b, 16, uint64(uint32(p.Status)))
	b = appendProtoVarint(b, 17, uint64(p.EndedAt))
	b = appendProtoString(b, 18, p.AnnounceRefsJSON)
	b = appendProtoString(b, 19, p.StageStateJSON)
	b = appendProtoBool(b, 20, p.TargetChatForcedOff)
	b = appendProtoVarint(b, 21, p.RevokeTargetSeq)
	return b
}

func UnmarshalConferenceSyncProto(data []byte) (*ConferenceSyncPayload, error) {
	p := &ConferenceSyncPayload{}
	i := 0
	for i < len(data) {
		key, n := consumeVarint(data[i:])
		if n <= 0 {
			return nil, errors.New("invalid protobuf key")
		}
		i += n
		field := int(key >> 3)
		wire := key & 7
		switch wire {
		case 0: // varint
			v, n := consumeVarint(data[i:])
			if n <= 0 {
				return nil, errors.New("invalid varint")
			}
			i += n
			switch field {
			case 2:
				p.Rev = int64(v)
			case 4:
				if v > math.MaxInt32 {
					return nil, errors.New("participant_count overflow")
				}
				p.ParticipantCount = int(v)
			case 5:
				p.GlobalAudioMuted = v != 0
			case 6:
				p.GlobalVideoOff = v != 0
			case 7:
				p.ChatMuted = v != 0
			case 8:
				p.Locked = v != 0
			case 14:
				p.TargetAudioForcedOff = v != 0
			case 15:
				p.TargetVideoForcedOff = v != 0
			case 16:
				p.Status = int32(v)
			case 17:
				p.EndedAt = int64(v)
			case 20:
				p.TargetChatForcedOff = v != 0
			case 21:
				p.RevokeTargetSeq = v
			}
		case 2: // length-delimited
			ln, n := consumeVarint(data[i:])
			if n <= 0 {
				return nil, errors.New("invalid length")
			}
			i += n
			if ln > uint64(len(data)-i) {
				return nil, errors.New("truncated string")
			}
			s := string(data[i : i+int(ln)])
			i += int(ln)
			switch field {
			case 1:
				p.RoomID = s
			case 3:
				p.AdminUserID = s
			case 9:
				p.PinnedUserID = s
			case 10:
				p.EgressID = s
			case 11:
				p.IngressID = s
			case 12:
				p.Event = s
			case 13:
				p.TargetUserID = s
			case 18:
				p.AnnounceRefsJSON = s
			case 19:
				p.StageStateJSON = s
			}
		default:
			return nil, errors.New("unsupported wire type")
		}
	}
	return p, nil
}

func appendProtoString(b []byte, field int, s string) []byte {
	if s == "" {
		return b
	}
	b = appendVarintRaw(b, uint64(field<<3|2))
	b = appendVarintRaw(b, uint64(len(s)))
	return append(b, s...)
}

func appendProtoVarint(b []byte, field int, v uint64) []byte {
	if v == 0 {
		return b
	}
	b = appendVarintRaw(b, uint64(field<<3|0))
	return appendVarintRaw(b, v)
}

func appendProtoBool(b []byte, field int, v bool) []byte {
	if !v {
		return b
	}
	b = appendVarintRaw(b, uint64(field<<3|0))
	return append(b, 1)
}

func appendVarintRaw(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func consumeVarint(b []byte) (uint64, int) {
	var x uint64
	var s uint
	for i := 0; i < len(b); i++ {
		if i == 10 {
			return 0, -1
		}
		c := b[i]
		if c < 0x80 {
			if i == 9 && c > 1 {
				return 0, -1
			}
			return x | uint64(c)<<s, i + 1
		}
		x |= uint64(c&0x7f) << s
		s += 7
	}
	return 0, -1
}
