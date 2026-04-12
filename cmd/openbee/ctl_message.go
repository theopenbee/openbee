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

func init() {
	ctlMessageSendCmd.Flags().StringVar(&msgSendMessageID, "message-id", "", "ID of the originating platform message (required)")
	ctlMessageSendCmd.Flags().BoolVar(&msgSendStdin, "stdin", false, "Read text content from stdin (use with heredoc)")
	ctlMessageSendCmd.Flags().StringVar(&msgSendMediaPath, "media-path", "", "Local file path to upload and send as media")
	ctlMessageSendCmd.MarkFlagRequired("message-id")

	ctlMessageCmd.AddCommand(ctlMessageSendCmd)
	ctlCmd.AddCommand(ctlMessageCmd)
}
