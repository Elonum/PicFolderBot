package yadisk

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const baseURL = "https://cloud-api.yandex.net/v1/disk/resources"
const yandexRetries = 3

type Client struct {
	token string
	http  *http.Client
}

func NewClient(token string, timeout time.Duration) *Client {
	return &Client{
		token: strings.TrimSpace(token),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

type itemEmbedded struct {
	Items []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"items"`
}

type resourceResp struct {
	Embedded itemEmbedded `json:"_embedded"`
}

type operationResp struct {
	Href string `json:"href"`
}

type apiError struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (c *Client) ListSubdirs(diskPath string) ([]string, error) {
	q := url.Values{}
	q.Set("path", diskPath)
	q.Set("limit", "200")
	q.Set("fields", "_embedded.items.name,_embedded.items.type")

	req, err := http.NewRequest(http.MethodGet, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.doWithRetry(func() (*http.Response, error) {
		return c.http.Do(req)
	})
	if err != nil {
		return nil, fmt.Errorf("yandex list error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}

	var payload resourceResp
	if err = json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode list response: %w", err)
	}

	out := make([]string, 0, len(payload.Embedded.Items))
	for _, it := range payload.Embedded.Items {
		if it.Type == "dir" {
			out = append(out, it.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (c *Client) EnsureDir(diskPath string) error {
	parts := strings.Split(strings.Trim(diskPath, "/"), "/")
	if len(parts) == 0 {
		return errors.New("invalid disk path")
	}
	current := parts[0]
	for i := 1; i < len(parts); i++ {
		current += "/" + parts[i]
		if err := c.mkdir(current); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) UploadFile(diskPath string, data []byte, mimeType string) error {
	u, err := c.getUploadURL(diskPath)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, u, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}

	resp, err := c.doWithRetry(func() (*http.Response, error) {
		return c.http.Do(req)
	})
	if err != nil {
		return fmt.Errorf("upload content error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("upload failed with status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) mkdir(diskPath string) error {
	q := url.Values{}
	q.Set("path", diskPath)

	req, err := http.NewRequest(http.MethodPut, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.doWithRetry(func() (*http.Response, error) {
		return c.http.Do(req)
	})
	if err != nil {
		return fmt.Errorf("mkdir error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict {
		return nil
	}
	return decodeAPIError(resp)
}

func (c *Client) getUploadURL(diskPath string) (string, error) {
	q := url.Values{}
	q.Set("path", diskPath)
	q.Set("overwrite", "true")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/upload?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	c.setAuth(req)

	resp, err := c.doWithRetry(func() (*http.Response, error) {
		return c.http.Do(req)
	})
	if err != nil {
		return "", fmt.Errorf("get upload url error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", decodeAPIError(resp)
	}

	var payload operationResp
	if err = json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Href == "" {
		return "", errors.New("empty upload href")
	}
	return payload.Href, nil
}

func (c *Client) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "OAuth "+c.token)
}

func decodeAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var api apiError
	if json.Unmarshal(body, &api) == nil && api.Message != "" {
		return fmt.Errorf("yandex api error (%d): %s", resp.StatusCode, api.Message)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return errors.New("yandex api unauthorized: check YANDEX_OAUTH_TOKEN")
	case http.StatusNotFound:
		return errors.New("yandex path not found")
	case http.StatusConflict:
		return errors.New("yandex conflict: resource already exists")
	case http.StatusRequestTimeout:
		return errors.New("yandex request timeout")
	default:
		return fmt.Errorf("yandex api error (%d)", resp.StatusCode)
	}
}

func (c *Client) doWithRetry(fn func() (*http.Response, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < yandexRetries; attempt++ {
		resp, err := fn()
		if err == nil {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				log.Printf("yadisk transient status=%d attempt=%d", resp.StatusCode, attempt+1)
				lastErr = fmt.Errorf("yandex api transient status %d", resp.StatusCode)
				resp.Body.Close()
				if attempt < yandexRetries-1 {
					time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
					continue
				}
			}
			return resp, nil
		}
		lastErr = err
		log.Printf("yadisk request retryable=%v attempt=%d err=%v", isTransientNetErr(err), attempt+1, err)
		if !isTransientNetErr(err) || attempt == yandexRetries-1 {
			return nil, err
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return nil, lastErr
}

func isTransientNetErr(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection aborted") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "failed to respond")
}
