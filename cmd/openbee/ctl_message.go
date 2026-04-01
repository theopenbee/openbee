package main

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/toolnames"
)

var ctlMessageCmd = &cobra.Command{Use: "message", Short: ""}

var (
	msgSendMessageID string
	msgSendContent   string
	msgSendMediaPath string
)

var ctlMessageSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to the user on the originating platform",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"message_id": msgSendMessageID}
		if msgSendContent != "" {
			a["content"] = strings.ReplaceAll(msgSendContent, `\n`, "\n")
		}
		if msgSendMediaPath != "" {
			a["media_path"] = msgSendMediaPath
		}
		return ctlRun(toolnames.SendMessage, a)
	},
}

func init() {
	ctlMessageSendCmd.Flags().StringVar(&msgSendMessageID, "message-id", "", "ID of the originating platform message (required)")
	ctlMessageSendCmd.Flags().StringVar(&msgSendContent, "content", "", "Text content to send")
	ctlMessageSendCmd.Flags().StringVar(&msgSendMediaPath, "media-path", "", "Local file path to upload and send as media")
	ctlMessageSendCmd.MarkFlagRequired("message-id")

	ctlMessageCmd.AddCommand(ctlMessageSendCmd)
	ctlCmd.AddCommand(ctlMessageCmd)
}
