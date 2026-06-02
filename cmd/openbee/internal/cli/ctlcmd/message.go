package ctlcmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newMessageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "message",
		Short: i18n.M.Cmd.CtlMessage.Short,
	}

	var (
		msgSendMessageID string
		msgSendStdin     bool
		msgSendMediaPath string
	)
	sendCmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to the user on the originating platform",
		RunE: func(c *cobra.Command, args []string) error {
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
	sendCmd.Flags().StringVar(&msgSendMessageID, "message-id", "", "ID of the originating platform message (required)")
	sendCmd.Flags().BoolVar(&msgSendStdin, "stdin", false, "Read text content from stdin (use with heredoc)")
	sendCmd.Flags().StringVar(&msgSendMediaPath, "media-path", "", "Local file path to upload and send as media")
	sendCmd.MarkFlagRequired("message-id")

	var (
		msgListSessionKey   string
		msgListPlatform     string
		msgListStatus       string
		msgListReceivedFrom int64
		msgListReceivedTo   int64
		msgListPage         int
		msgListPageSize     int
	)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List platform messages with optional filters",
		RunE: func(c *cobra.Command, args []string) error {
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
			if msgListPage > 0 {
				a["page"] = msgListPage
			}
			if msgListPageSize > 0 {
				a["page_size"] = msgListPageSize
			}
			return ctlRun(utils.ListMessages, a)
		},
	}
	listCmd.Flags().StringVar(&msgListSessionKey, "session-key", "", "Filter by session key")
	listCmd.Flags().StringVar(&msgListPlatform, "platform", "", "Filter by platform (e.g. feishu, local)")
	listCmd.Flags().StringVar(&msgListStatus, "status", "", "Filter by status (received, feeding, bee_processed, merged, failed)")
	listCmd.Flags().Int64Var(&msgListReceivedFrom, "received-from", 0, "Filter received_at >= value (Unix ms)")
	listCmd.Flags().Int64Var(&msgListReceivedTo, "received-to", 0, "Filter received_at <= value (Unix ms)")
	listCmd.Flags().IntVar(&msgListPage, "page", 0, "Page number (default: 1)")
	listCmd.Flags().IntVar(&msgListPageSize, "page-size", 0, "Page size (default: 50, max: 100)")

	var (
		msgListOutSessionKey string
		msgListOutPlatform   string
		msgListOutStatus     string
		msgListOutSourceType string
		msgListOutSourceID   string
		msgListOutSentFrom   int64
		msgListOutSentTo     int64
		msgListOutPage       int
		msgListOutPageSize   int
	)
	listOutboundCmd := &cobra.Command{
		Use:   "list-outbound",
		Short: "List outbound (sent) messages with optional filters",
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{}
			if msgListOutSessionKey != "" {
				a["session_key"] = msgListOutSessionKey
			}
			if msgListOutPlatform != "" {
				a["platform"] = msgListOutPlatform
			}
			if msgListOutStatus != "" {
				a["status"] = msgListOutStatus
			}
			if msgListOutSourceType != "" {
				a["source_type"] = msgListOutSourceType
			}
			if msgListOutSourceID != "" {
				a["source_id"] = msgListOutSourceID
			}
			if msgListOutSentFrom > 0 {
				a["sent_at_from"] = msgListOutSentFrom
			}
			if msgListOutSentTo > 0 {
				a["sent_at_to"] = msgListOutSentTo
			}
			if msgListOutPage > 0 {
				a["page"] = msgListOutPage
			}
			if msgListOutPageSize > 0 {
				a["page_size"] = msgListOutPageSize
			}
			return ctlRun(utils.ListOutboundMessages, a)
		},
	}
	listOutboundCmd.Flags().StringVar(&msgListOutSessionKey, "session-key", "", "Filter by session key")
	listOutboundCmd.Flags().StringVar(&msgListOutPlatform, "platform", "", "Filter by platform (e.g. feishu, local)")
	listOutboundCmd.Flags().StringVar(&msgListOutStatus, "status", "", "Filter by status (sent, failed)")
	listOutboundCmd.Flags().StringVar(&msgListOutSourceType, "source-type", "", "Filter by source type (bee, worker, system)")
	listOutboundCmd.Flags().StringVar(&msgListOutSourceID, "source-id", "", "Filter by source ID")
	listOutboundCmd.Flags().Int64Var(&msgListOutSentFrom, "sent-from", 0, "Filter sent_at >= value (Unix ms)")
	listOutboundCmd.Flags().Int64Var(&msgListOutSentTo, "sent-to", 0, "Filter sent_at <= value (Unix ms)")
	listOutboundCmd.Flags().IntVar(&msgListOutPage, "page", 0, "Page number (default: 1)")
	listOutboundCmd.Flags().IntVar(&msgListOutPageSize, "page-size", 0, "Page size (default: 50, max: 100)")

	cmd.AddCommand(sendCmd, listCmd, listOutboundCmd)
	return cmd
}
