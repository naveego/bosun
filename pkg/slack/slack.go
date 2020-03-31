package slack

import (
	"bytes"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"os"

	"encoding/json"
	"errors"
	"net/http"
	"time"
)


type Notification struct {
	WebhookURL string
	Message string
	IconURL string
	IconEmoji string
}

func (s Notification) WithMessage(format string, args ...interface{}) Notification {
	s.Message = fmt.Sprintf(format, args...)
	return s
}

type slackRequestBody struct {
	Text string `json:"text"`
	Username string `json:"username,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
	IconEmoji string `json:"icon_emoji,omitempty"`
}

func (s Notification) Send() {
	webhookURL := s.WebhookURL
	shouldSend := true
	if webhookURL == "" {
		webhookURL, shouldSend = os.LookupEnv("SLACK_WEBHOOK")
	}
	if !shouldSend {
		pkg.Log.Infof("Skipping slack notification (set SLACK_WEBHOOK to enable): %s", s.Message)
		return
	}

	body := slackRequestBody{
		Text: s.Message,
		Username: os.Getenv("USER"),
		IconURL:s.IconURL,
		IconEmoji:s.IconEmoji,
	}

	slackBody, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(slackBody))
	if err != nil {
		pkg.Log.WithError(err).Error("Slack notification failed.")
		return
	}

	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		pkg.Log.WithError(err).Error("Slack notification failed.")
		return
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if buf.String() != "ok" {
		pkg.Log.WithError(errors.New(buf.String())).Error("Slack notification failed.")
	}
}