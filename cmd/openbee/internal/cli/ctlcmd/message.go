package ctlcmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func newMessageCommand(run Runner) *cobra.Command {
	subs := i18n.M.Cmd.CtlMessage
	cmd := &cobra.Command{
		Use:   "message",
		Short: subs.Short,
	}
	cmd.AddCommand(
		newMessageSendCommand(run, subs.Sub("send")),
		newMessageListCommand(run, subs.Sub("list"), false),
		newMessageListCommand(run, subs.Sub("list-outbound"), true),
	)
	return cmd
}

func newMessageSendCommand(run Runner, short string) *cobra.Command {
	var (
		messageID string
		fromStdin bool
		mediaPath string
	)
	cmd := &cobra.Command{
		Use:   "send",
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{"message_id": messageID}
			if fromStdin {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				if len(b) > 0 {
					a["content"] = string(b)
				}
			}
			setIfNonEmpty(a, "media_path", mediaPath)
			return run(utils.SendMessage, a)
		},
	}
	cmd.Flags().StringVar(&messageID, "message-id", "", "ID of the originating platform message (required)")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Read text content from stdin (use with heredoc)")
	cmd.Flags().StringVar(&mediaPath, "media-path", "", "Local file path to upload and send as media")
	cmd.MarkFlagRequired("message-id")
	return cmd
}

// newMessageListCommand builds either the inbound `list` or outbound `list-outbound`
// subcommand; the two share most filters but differ in the time-field flags
// (received_at vs sent_at) and a couple of outbound-only filters.
func newMessageListCommand(run Runner, short string, outbound bool) *cobra.Command {
	var (
		sessionKey string
		platform   string
		status     string
		sourceType string
		sourceID   string
		timeFrom   int64
		timeTo     int64
		page       int
		pageSize   int
	)

	use := "list"
	tool := utils.ListMessages
	fromFlag, toFlag := "received-from", "received-to"
	fromUsage := "Filter received_at >= value (Unix ms)"
	toUsage := "Filter received_at <= value (Unix ms)"
	fromKey, toKey := "received_at_from", "received_at_to"
	statusUsage := "Filter by status (received, feeding, bee_processed, merged, failed)"

	if outbound {
		use = "list-outbound"
		tool = utils.ListOutboundMessages
		fromFlag, toFlag = "sent-from", "sent-to"
		fromUsage = "Filter sent_at >= value (Unix ms)"
		toUsage = "Filter sent_at <= value (Unix ms)"
		fromKey, toKey = "sent_at_from", "sent_at_to"
		statusUsage = "Filter by status (sent, failed)"
	}

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			a := map[string]any{}
			setIfNonEmpty(a, "session_key", sessionKey)
			setIfNonEmpty(a, "platform", platform)
			setIfNonEmpty(a, "status", status)
			if outbound {
				setIfNonEmpty(a, "source_type", sourceType)
				setIfNonEmpty(a, "source_id", sourceID)
			}
			setIfPositiveInt64(a, fromKey, timeFrom)
			setIfPositiveInt64(a, toKey, timeTo)
			setIfPositive(a, "page", page)
			setIfPositive(a, "page_size", pageSize)
			return run(tool, a)
		},
	}
	cmd.Flags().StringVar(&sessionKey, "session-key", "", "Filter by session key")
	cmd.Flags().StringVar(&platform, "platform", "", "Filter by platform (e.g. feishu, local)")
	cmd.Flags().StringVar(&status, "status", "", statusUsage)
	if outbound {
		cmd.Flags().StringVar(&sourceType, "source-type", "", "Filter by source type (bee, worker, system)")
		cmd.Flags().StringVar(&sourceID, "source-id", "", "Filter by source ID")
	}
	cmd.Flags().Int64Var(&timeFrom, fromFlag, 0, fromUsage)
	cmd.Flags().Int64Var(&timeTo, toFlag, 0, toUsage)
	cmd.Flags().IntVar(&page, "page", 0, "Page number (default: 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "Page size (default: 50, max: 100)")
	return cmd
}
