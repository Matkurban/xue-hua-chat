package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// conferenceLiveKit is the LiveKit surface used by ConferenceManager (injectable for tests).
type conferenceLiveKit interface {
	ClientURL() string
	APIKey() string
	APISecret() string
	CreateRoom(ctx context.Context, roomID string, maxParticipants uint32) error
	DeleteRoom(ctx context.Context, roomID string) error
	RemoveParticipant(ctx context.Context, roomID, identity string) error
	UpdateParticipantPermission(
		ctx context.Context, roomID, identity string,
		canPublish, canPublishData, roomAdmin, allowMic, allowCamera bool,
		mem *ConferenceMember,
	) error
	MuteMediaKind(ctx context.Context, roomID, identity string, video bool, muted bool) error
	StartRoomCompositeEgress(ctx context.Context, roomID, filepath string) (string, error)
	StopEgress(ctx context.Context, egressID string) error
	CreateRTMPIngress(ctx context.Context, roomID, name string) (*livekit.IngressInfo, error)
	CreateWHIPIngress(ctx context.Context, roomID, name string) (*livekit.IngressInfo, error)
	DeleteIngress(ctx context.Context, ingressID string) error
	GenerateToken(roomID, userID, role string, canPublishData, allowMic, allowCamera bool) (string, error)
	// SendData pushes a payload to every participant in the room over the data channel.
	// One API call fans out to the whole room, which is why room-wide notifications use
	// this instead of a per-user IM business notification.
	SendData(ctx context.Context, roomID string, payload []byte) error
}

// conferenceLK wraps LiveKit Room / Egress / Ingress clients.
type conferenceLK struct {
	api    *lksdk.LiveKitAPI
	key    string
	secret string
	url    string // client-facing ws(s) URL
}

func (c *conferenceLK) ClientURL() string { return c.url }
func (c *conferenceLK) APIKey() string    { return c.key }
func (c *conferenceLK) APISecret() string { return c.secret }

func newConferenceLK(url, key, secret string) (*conferenceLK, error) {
	apiURL := liveKitHTTPURL(url)
	api, err := lksdk.NewLiveKitAPI(
		lksdk.WithURL(apiURL),
		lksdk.WithAPIKey(key, secret),
	)
	if err != nil {
		return nil, err
	}
	return &conferenceLK{api: api, key: key, secret: secret, url: url}, nil
}

func liveKitHTTPURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.Replace(u, "wss://", "https://", 1)
	u = strings.Replace(u, "ws://", "http://", 1)
	return u
}

func (c *conferenceLK) CreateRoom(ctx context.Context, roomID string, maxParticipants uint32) error {
	_, err := c.api.Room().CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:            roomID,
		EmptyTimeout:    300,
		MaxParticipants: maxParticipants,
		Metadata:        `{"mode":"conference"}`,
	})
	return err
}

func (c *conferenceLK) DeleteRoom(ctx context.Context, roomID string) error {
	_, err := c.api.Room().DeleteRoom(ctx, &livekit.DeleteRoomRequest{Room: roomID})
	return err
}

func (c *conferenceLK) RemoveParticipant(ctx context.Context, roomID, identity string) error {
	_, err := c.api.Room().RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room: roomID, Identity: identity,
	})
	return err
}

func (c *conferenceLK) SendData(ctx context.Context, roomID string, payload []byte) error {
	_, err := c.api.Room().SendData(ctx, &livekit.SendDataRequest{
		Room: roomID,
		Data: payload,
		Kind: livekit.DataPacket_RELIABLE,
	})
	return err
}

// conferenceParticipantMeta is compact UTF-8 JSON on LiveKit Participant.metadata
// for late-join UI recovery (authoritative state remains Redis + conference_sync).
//
// State flags must never use omitempty: clients treat the metadata as a full
// snapshot, so a dropped `false` would leave a lifted restriction stuck on.
type conferenceParticipantMeta struct {
	Role           string `json:"role,omitempty"`
	AudioForcedOff bool   `json:"audioForcedOff"`
	VideoForcedOff bool   `json:"videoForcedOff"`
	ChatForcedOff  bool   `json:"chatForcedOff"`
	OnStage        bool   `json:"onStage"`
	CameraOn       bool   `json:"cameraOn"`
	SpeakPending   bool   `json:"speakPending"`
	CanPublish     bool   `json:"canPublish"`
	RoomAdmin      bool   `json:"roomAdmin"`
	AllowMic       bool   `json:"allowMic"`
	AllowCamera    bool   `json:"allowCamera"`
}

func buildConferenceParticipantMetadata(
	mem *ConferenceMember, canPublish, roomAdmin, allowMic, allowCamera bool,
) string {
	meta := conferenceParticipantMeta{
		CanPublish:  canPublish,
		RoomAdmin:   roomAdmin,
		AllowMic:    allowMic,
		AllowCamera: allowCamera,
	}
	if mem != nil {
		meta.Role = mem.Role
		meta.AudioForcedOff = mem.AudioForcedOff
		meta.VideoForcedOff = mem.VideoForcedOff
		meta.ChatForcedOff = mem.ChatForcedOff
		meta.OnStage = mem.OnStage
		meta.CameraOn = mem.CameraOn
		meta.SpeakPending = mem.SpeakPending
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Sprintf(`{"canPublish":%v,"roomAdmin":%v}`, canPublish, roomAdmin)
	}
	return string(b)
}

// UpdateParticipantPermission updates LiveKit grants. When canPublish is true and
// allowMic/allowCamera restrict sources, CanPublishSources is set so the SFU
// rejects republish even if the client ignores forced-mute UI.
func (c *conferenceLK) UpdateParticipantPermission(
	ctx context.Context, roomID, identity string,
	canPublish, canPublishData, roomAdmin, allowMic, allowCamera bool,
	mem *ConferenceMember,
) error {
	perm := &livekit.ParticipantPermission{
		CanSubscribe:      true,
		CanPublish:        canPublish,
		CanPublishData:    canPublishData,
		CanUpdateMetadata: true,
	}
	if canPublish {
		if sources := publishSources(allowMic, allowCamera); sources != nil {
			perm.CanPublishSources = sources
		}
	}
	_, err := c.api.Room().UpdateParticipant(ctx, &livekit.UpdateParticipantRequest{
		Room:       roomID,
		Identity:   identity,
		Permission: perm,
		Metadata:   buildConferenceParticipantMetadata(mem, canPublish, roomAdmin, allowMic, allowCamera),
	})
	return err
}

// publishSources returns nil when unrestricted (all sources). Otherwise an
// explicit allow-list. Empty slice means canPublish but no source allowed.
func publishSources(allowMic, allowCamera bool) []livekit.TrackSource {
	if allowMic && allowCamera {
		return nil
	}
	out := make([]livekit.TrackSource, 0, 4)
	if allowCamera {
		out = append(out, livekit.TrackSource_CAMERA, livekit.TrackSource_SCREEN_SHARE)
	}
	if allowMic {
		out = append(out, livekit.TrackSource_MICROPHONE, livekit.TrackSource_SCREEN_SHARE_AUDIO)
	}
	return out
}

func (c *conferenceLK) MuteTrack(ctx context.Context, roomID, identity, trackSID string, muted bool) error {
	_, err := c.api.Room().MutePublishedTrack(ctx, &livekit.MuteRoomTrackRequest{
		Room: roomID, Identity: identity, TrackSid: trackSID, Muted: muted,
	})
	return err
}

func (c *conferenceLK) ListParticipantTracks(ctx context.Context, roomID, identity string) ([]*livekit.TrackInfo, error) {
	p, err := c.api.Room().GetParticipant(ctx, &livekit.RoomParticipantIdentity{
		Room: roomID, Identity: identity,
	})
	if err != nil {
		return nil, err
	}
	return p.Tracks, nil
}

func (c *conferenceLK) MuteMediaKind(ctx context.Context, roomID, identity string, video bool, muted bool) error {
	tracks, err := c.ListParticipantTracks(ctx, roomID, identity)
	if err != nil {
		return err
	}
	want := livekit.TrackType_AUDIO
	if video {
		want = livekit.TrackType_VIDEO
	}
	for _, t := range tracks {
		if t.Type != want {
			continue
		}
		if err := c.MuteTrack(ctx, roomID, identity, t.Sid, muted); err != nil {
			return err
		}
	}
	return nil
}

func (c *conferenceLK) StartRoomCompositeEgress(ctx context.Context, roomID, filepath string) (string, error) {
	info, err := c.api.Egress().StartRoomCompositeEgress(ctx, &livekit.RoomCompositeEgressRequest{
		RoomName: roomID,
		Layout:   "grid",
		FileOutputs: []*livekit.EncodedFileOutput{
			{
				FileType: livekit.EncodedFileType_MP4,
				Filepath: filepath,
			},
		},
	})
	if err != nil {
		return "", err
	}
	return info.EgressId, nil
}

func (c *conferenceLK) StopEgress(ctx context.Context, egressID string) error {
	_, err := c.api.Egress().StopEgress(ctx, &livekit.StopEgressRequest{EgressId: egressID})
	return err
}

func (c *conferenceLK) CreateRTMPIngress(ctx context.Context, roomID, name string) (*livekit.IngressInfo, error) {
	return c.CreateIngress(ctx, roomID, name, livekit.IngressInput_RTMP_INPUT)
}

func (c *conferenceLK) CreateWHIPIngress(ctx context.Context, roomID, name string) (*livekit.IngressInfo, error) {
	return c.CreateIngress(ctx, roomID, name, livekit.IngressInput_WHIP_INPUT)
}

func (c *conferenceLK) CreateIngress(ctx context.Context, roomID, name string, inputType livekit.IngressInput) (*livekit.IngressInfo, error) {
	return c.api.Ingress().CreateIngress(ctx, &livekit.CreateIngressRequest{
		InputType:           inputType,
		Name:                name,
		RoomName:            roomID,
		ParticipantIdentity: "ingress-" + name,
		ParticipantName:     name,
	})
}

func (c *conferenceLK) DeleteIngress(ctx context.Context, ingressID string) error {
	_, err := c.api.Ingress().DeleteIngress(ctx, &livekit.DeleteIngressRequest{IngressId: ingressID})
	return err
}

func (c *conferenceLK) GenerateToken(
	roomID, userID, role string, canPublishData, allowMic, allowCamera bool,
) (string, error) {
	at := auth.NewAccessToken(c.key, c.secret)
	canPublish := allowMic || allowCamera
	grant := &auth.VideoGrant{
		RoomJoin:       true,
		Room:           roomID,
		CanPublish:     &canPublish,
		CanSubscribe:   boolPtr(true),
		CanPublishData: &canPublishData,
	}
	if role == ConferenceRoleAdmin {
		grant.RoomAdmin = true
	}
	if canPublish {
		if sources := publishSources(allowMic, allowCamera); sources != nil {
			grant.SetCanPublishSources(sources)
		}
	}
	at.AddGrant(grant).
		SetIdentity(userID).
		SetName(userID).
		SetValidFor(24 * time.Hour)
	return at.ToJWT()
}

func boolPtr(v bool) *bool { return &v }
