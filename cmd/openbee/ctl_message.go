package main

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlMessageCmd = &cobra.Command{Use: "message", Short: ""}

var (
	msgSendMessageID string
	msgSendStdin     bool
	msgSendMediaPath string
)

var (
	msgListSessionKey    string
	msgListPlatform      string
	msgListStatus        string
	msgListReceivedFrom  int64
	msgListReceivedTo    int64
)

var ctlMessageSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to the user on the originating platform",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"message_id": msgSendMessageID}
		if msgSendStdin {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			if len(b) > 0 {
				a["content"] = string(b)
			}
		}
		if msgSendMediaPath != "" {
			a["media_path"] = msgSendMediaPath
		}
		return ctlRun(utils.SendMessage, a)
	},
}

var ctlMessageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List platform messages with optional filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{}
		if msgListSessionKey != "" {
			a["session_key"] = msgListSessionKey
		}
		if msgListPlatform != "" {
			a["platform"] = msgListPlatform
		}
		if msgListStatus != "" {
			a["status"] = msgListStatus
		}
		if msgListReceivedFrom > 0 {
			a["received_at_from"] = msgListReceivedFrom
		}
		if msgListReceivedTo > 0 {
			a["received_at_to"] = msgListReceivedTo
		}
		return ctlRun(utils.ListMessages, a)
	},
}

func init() {
	ctlMessageSendCmd.Flags().StringVar(&msgSendMessageID, "message-id", "", "ID of the originating platform message (required)")
	ctlMessageSendCmd.Flags().BoolVar(&msgSendStdin, "stdin", false, "Read text content from stdin (use with heredoc)")
	ctlMessageSendCmd.Flags().StringVar(&msgSendMediaPath, "media-path", "", "Local file path to upload and send as media")
	ctlMessageSendCmd.MarkFlagRequired("message-id")

	ctlMessageListCmd.Flags().StringVar(&msgListSessionKey, "session-key", "", "Filter by session key")
	ctlMessageListCmd.Flags().StringVar(&msgListPlatform, "platform", "", "Filter by platform (e.g. feishu, local)")
	ctlMessageListCmd.Flags().StringVar(&msgListStatus, "status", "", "Filter by status (received, feeding, bee_processed, merged, failed)")
	ctlMessageListCmd.Flags().Int64Var(&msgListReceivedFrom, "received-from", 0, "Filter received_at >= value (Unix ms)")
	ctlMessageListCmd.Flags().Int64Var(&msgListReceivedTo, "received-to", 0, "Filter received_at <= value (Unix ms)")

	ctlMessageCmd.AddCommand(ctlMessageSendCmd, ctlMessageListCmd)
	ctlCmd.AddCommand(ctlMessageCmd)
}
