// Package webhook delivers call events to the Chatwoot backend.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// defaultTimeout is the HTTP client timeout for webhook delivery.
	defaultTimeout = 10 * time.Second

	// HeaderGatewaySecret is the header used to authenticate gateway requests.
	HeaderGatewaySecret = "X-Gateway-Secret"

	contentTypeJSON = "application/json"
)

// Client delivers webhook events to a configured endpoint.
type Client struct {
	url    string
	secret string
	http   *http.Client
}

// NewClient creates a webhook client targeting the given URL.
func NewClient(url, secret string) *Client {
	return &Client{
		url:    url,
		secret: secret,
		http: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// Send delivers an event with the given data payload.
func (c *Client) Send(event string, data map[string]any) error {
	return c.SendContext(context.Background(), event, data)
}

// SendContext delivers an event with the given data payload, respecting the context.
func (c *Client) SendContext(ctx context.Context, event string, data map[string]any) error {
	payload := map[string]any{
		"event": event,
		"data":  data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set(HeaderGatewaySecret, c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook %s: %w", event, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("webhook %s returned status %d", event, resp.StatusCode)
	}

	log.Debug().Str("event", event).Int("status", resp.StatusCode).Msg("webhook delivered")
	return nil
}

// UploadRecording sends a finished recording file to Chatwoot for Active Storage attachment.
func (c *Client) UploadRecording(callID, recordingPath string) error {
	return c.UploadRecordingContext(context.Background(), callID, recordingPath)
}

// UploadRecordingContext sends a finished recording file to Chatwoot, respecting the context.
func (c *Client) UploadRecordingContext(ctx context.Context, callID, recordingPath string) error {
	bodyReader, contentType, err := buildRecordingBody(callID, recordingPath)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.recordingURL(), bodyReader)
	if err != nil {
		return fmt.Errorf("create recording upload request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set(HeaderGatewaySecret, c.secret)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload recording for %s: %w", callID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("recording upload returned status %d", resp.StatusCode)
	}

	log.Debug().Str("call_id", callID).Int("status", resp.StatusCode).Msg("recording uploaded")
	return nil
}

func (c *Client) recordingURL() string {
	parsedURL, err := url.Parse(c.url)
	if err != nil {
		return strings.TrimRight(c.url, "/") + "/recordings"
	}

	trimmedPath := strings.TrimRight(parsedURL.Path, "/")
	if strings.HasSuffix(trimmedPath, "/events") {
		parsedURL.Path = strings.TrimSuffix(trimmedPath, "/events") + "/recordings"
	} else {
		parsedURL.Path = trimmedPath + "/recordings"
	}

	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""
	return parsedURL.String()
}

func buildRecordingBody(callID, recordingPath string) (io.Reader, string, error) {
	file, err := os.Open(recordingPath)
	if err != nil {
		return nil, "", fmt.Errorf("open recording %s: %w", recordingPath, err)
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	go func() {
		defer file.Close()
		defer pipeWriter.Close()

		if err := writer.WriteField("call_id", callID); err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("write call_id field: %w", err))
			return
		}

		part, err := writer.CreateFormFile("recording", filepath.Base(recordingPath))
		if err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("create recording part: %w", err))
			return
		}

		if _, err := io.Copy(part, file); err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("stream recording body: %w", err))
			return
		}

		if err := writer.Close(); err != nil {
			_ = pipeWriter.CloseWithError(fmt.Errorf("close multipart writer: %w", err))
		}
	}()

	return pipeReader, writer.FormDataContentType(), nil
}
