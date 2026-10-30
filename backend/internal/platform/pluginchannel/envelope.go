package pluginchannel

import (
	"encoding/json"

	"github.com/theopenbee/openbee/internal/platform"
)

const envelopeVersion = 1

type envelope struct {
	V         int             `json:"v"`
	Raw       string          `json:"raw,omitempty"`
	MessageID string          `json:"message_id,omitempty"`
	Context   json.RawMessage `json:"context,omitempty"`
}

func EncodeEnvelope(raw, messageID string, ctx json.RawMessage) string {
	if len(ctx) > 0 && !json.Valid(ctx) {
		ctx = nil
	}
	b, _ := json.Marshal(envelope{V: envelopeVersion, Raw: raw, MessageID: messageID, Context: ctx})
	return string(b)
}

func DecodeEnvelope(s string) (raw, messageID string, ctx json.RawMessage) {
	e, _ := parseEnvelope(s)
	return e.Raw, e.MessageID, e.Context
}

func parseEnvelope(s string) (envelope, bool) {
	var e envelope
	if json.Unmarshal([]byte(s), &e) != nil || e.V != envelopeVersion {
		return envelope{}, false
	}
	return e, true
}

func replyTo(stored string) *ReplyTo {
	if stored == "" {
		return nil
	}
	e, ok := parseEnvelope(stored)
	if !ok {
		return &ReplyTo{Raw: stored}
	}
	if e.Raw == "" && e.MessageID == "" {
		return nil
	}
	return &ReplyTo{MessageID: e.MessageID, Raw: e.Raw}
}

func ExtractContextFor(channelID string) func(raw string) string {
	return func(raw string) string {
		_, _, ctx := DecodeEnvelope(raw)
		if len(ctx) == 0 || string(ctx) == "null" {
			return ""
		}
		return platform.MarshalContext(channelID, ctx)
	}
}
