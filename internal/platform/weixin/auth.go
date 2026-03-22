package weixin

import (
	"context"
	"fmt"
	"os"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
)

// QRLoginResult contains the result of a successful QR code login.
type QRLoginResult struct {
	Token   string
	BotID   string
	BaseUrl string
	UserID  string
}

// QRLogin performs the interactive QR code login flow.
// It renders a QR code in the terminal and waits for the user to scan it.
// Returns the login result or an error if the flow times out or fails.
func QRLogin(ctx context.Context, baseUrl string) (*QRLoginResult, error) {
	client := NewAPIClient(baseUrl, "", "")

	// Get QR code
	qrResp, err := client.GetBotQRCode(ctx, "3")
	if err != nil {
		return nil, fmt.Errorf("get QR code: %w", err)
	}
	if qrResp.QRCode == "" {
		return nil, fmt.Errorf("empty QR code in response")
	}

	// Render QR code in terminal
	fmt.Println("\nScan this QR code with WeChat to login:")
	qrterminal.GenerateWithConfig(qrResp.QRCode, qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
	})
	fmt.Println("\nWaiting for scan... (timeout: 5 minutes)")

	// Poll for scan status
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		statusResp, err := client.GetQRCodeStatus(ctx, qrResp.QRCode)
		if err != nil {
			// Timeout is expected for long-poll, retry
			continue
		}
		if statusResp.Status == 2 { // confirmed
			return &QRLoginResult{
				Token:   statusResp.BotToken,
				BotID:   statusResp.BotID,
				BaseUrl: statusResp.BaseUrl,
				UserID:  statusResp.UserID,
			}, nil
		}
		if statusResp.Status == 1 {
			fmt.Println("QR code scanned, waiting for confirmation...")
		}
	}
	return nil, fmt.Errorf("QR login timed out after 5 minutes")
}
