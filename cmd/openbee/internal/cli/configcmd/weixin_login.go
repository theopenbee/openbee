package configcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func runWeixinQRLogin() (token, userID, baseURL string, err error) {
	const defaultBase = "https://ilinkai.weixin.qq.com"
	client := &http.Client{Timeout: 15 * time.Second}

	// Step 1: Get QR code
	resp, err := client.Get(defaultBase + "/ilink/bot/get_bot_qrcode?bot_type=3")
	if err != nil {
		return "", "", "", fmt.Errorf("get qrcode: %w", err)
	}
	defer resp.Body.Close()

	var qrResp struct {
		QRCode           string `json:"qrcode"`
		QRCodeImgContent string `json:"qrcode_img_content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&qrResp); err != nil {
		return "", "", "", fmt.Errorf("decode qrcode response: %w", err)
	}

	// Step 2: Display QR code in terminal
	fmt.Println(i18n.M.Output.Weixin.ScanQR)
	qrterminal.GenerateWithConfig(qrResp.QRCodeImgContent, qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    os.Stdout,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
	})

	// Step 3: Poll for scan status (max 5 minutes total, max 3 timeouts)
	fmt.Println(i18n.M.Output.Weixin.Waiting)
	pollClient := &http.Client{Timeout: 40 * time.Second}
	deadline := time.Now().Add(5 * time.Minute)
	for attempt := 0; attempt < 3 && time.Now().Before(deadline); attempt++ {
		req, _ := http.NewRequest(http.MethodGet,
			fmt.Sprintf("%s/ilink/bot/get_qrcode_status?qrcode=%s", defaultBase, url.QueryEscape(qrResp.QRCode)), nil)
		req.Header.Set("iLink-App-ClientVersion", "1")

		pollResp, err := pollClient.Do(req)
		if err != nil {
			fmt.Printf(i18n.M.Output.Weixin.PollFailed+"\n", attempt+1, err)
			continue
		}

		var statusResp struct {
			Status      string `json:"status"`
			BotToken    string `json:"bot_token"`
			ILinkBotID  string `json:"ilink_bot_id"`
			BaseURL     string `json:"baseurl"`
			ILinkUserID string `json:"ilink_user_id"`
		}
		if err := json.NewDecoder(pollResp.Body).Decode(&statusResp); err != nil {
			pollResp.Body.Close()
			fmt.Printf(i18n.M.Output.Weixin.PollInvalid+"\n", attempt+1, err)
			continue
		}
		pollResp.Body.Close()

		switch statusResp.Status {
		case "confirmed":
			return statusResp.BotToken, statusResp.ILinkUserID, statusResp.BaseURL, nil
		case "scaned":
			fmt.Println(i18n.M.Output.Weixin.Scanned)
			attempt--               // don't count scanned as a failed attempt
			time.Sleep(time.Second) // prevent tight loop if server responds instantly
		case "expired":
			return "", "", "", errors.New(i18n.M.Output.Weixin.QRExpired)
		case "wait":
			fmt.Println(i18n.M.Output.Weixin.StillWaiting)
		}
	}
	return "", "", "", errors.New(i18n.M.Output.Weixin.QRTimeout)
}
