package pi

import (
	"strings"
	"testing"
)

func BenchmarkStripThinkingSignature_NoMatch(b *testing.B) {
	line := []byte(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`)
	b.ResetTimer()
	b.SetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		stripThinkingSignature(line)
	}
}

func BenchmarkStripThinkingSignature_OneMessageOneBlock(b *testing.B) {
	body := `{"type":"thinking","thinkingSignature":"` + strings.Repeat("x", 256) + `","thinking":"some"}`
	line := []byte(`{"type":"message_end","message":{"role":"assistant","content":[` + body + `]}}`)
	b.ResetTimer()
	b.SetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		stripThinkingSignature(line)
	}
}

func BenchmarkStripThinkingSignature_AgentEndManyMessages(b *testing.B) {
	body := `{"type":"thinking","thinkingSignature":"` + strings.Repeat("x", 256) + `","thinking":"some"}`
	msg := `{"role":"assistant","content":[` + body + `]}`
	var msgs []string
	for i := 0; i < 8; i++ {
		msgs = append(msgs, msg)
	}
	line := []byte(`{"type":"agent_end","messages":[` + strings.Join(msgs, ",") + `]}`)
	b.ResetTimer()
	b.SetBytes(int64(len(line)))
	for i := 0; i < b.N; i++ {
		stripThinkingSignature(line)
	}
}
