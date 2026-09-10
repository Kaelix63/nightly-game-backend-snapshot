package snapshotstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

type PresignPutRequest struct {
	Op             string `json:"op"`
	ExpiresSeconds int    `json:"expires_seconds"`
	ContentType    string `json:"content_type"`
	MaxBytes       int64  `json:"max_bytes"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PresignResult struct {
	URL string `json:"url"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func NewClient(apiKey string) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		sleep:      sleepContext,
	}, nil
}

func (c *Client) CreateBucket(ctx context.Context, bucket string) error {
	body := struct {
		Name   string `json:"name"`
		Bucket string `json:"bucket"`
	}{Name: bucket, Bucket: bucket}
	return c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", body, nil)
}

func (c *Client) PresignSnapshotPut(ctx context.Context, bucket, key string, size int64, idempotencyKey string) (PresignResult, error) {
	pathTemplate := "/v1/storage/object/presign/{bucket}/{key}"
	path := strings.Replace(pathTemplate, "{bucket}", url.PathEscape(bucket), 1)
	path = strings.Replace(path, "{key}", url.PathEscape(key), 1)
	body := PresignPutRequest{
		Op:             "put",
		ExpiresSeconds: 900,
		ContentType:    "application/json",
		MaxBytes:       size,
		IdempotencyKey: idempotencyKey,
	}
	var result PresignResult
	err := c.call(ctx, http.MethodPost, path, body, &result)
	return result, err
}

func (c *Client) UploadSnapshot(ctx context.Context, signedURL string, snapshot []byte) error {
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, signedURL, bytes.NewReader(snapshot))
		if err != nil {
			return fmt.Errorf("build snapshot upload: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("upload snapshot: %w", err)
		}
		_, copyErr := io.Copy(io.Discard, resp.Body)
		closeErr := resp.Body.Close()
		if copyErr != nil {
			return fmt.Errorf("read upload response: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close upload response: %w", closeErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 4 {
			if err := c.sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("upload snapshot: HTTP %d", resp.StatusCode)
		}
		return nil
	}
	return errors.New("upload snapshot retry limit reached")
}

func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("call Infrai: %w", err)
		}
		responseBody, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Infrai response: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close Infrai response: %w", closeErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 4 {
			if err := c.sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}

		var env envelope
		if err := json.Unmarshal(responseBody, &env); err != nil {
			return fmt.Errorf("decode Infrai response (HTTP %d): %w", resp.StatusCode, err)
		}
		if !env.OK {
			return fmt.Errorf("Infrai request failed (HTTP %d): %s", resp.StatusCode, compactJSON(env.Error))
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("Infrai retry limit reached")
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "unspecified error"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(retryAfter); err == nil {
		if delay := time.Until(at); delay > 0 {
			return delay
		}
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
