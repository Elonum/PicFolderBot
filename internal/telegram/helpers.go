package telegram

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"PicFolderBot/internal/observability"
)

func extractEditID(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	return values[0]
}

func (b *Bot) getSession(chatID int64) *sessionState {
	return b.sessionStore.Get(chatID)
}

func (b *Bot) setSession(chatID int64, state *sessionState) {
	b.sessionStore.Set(chatID, state)
}

func (b *Bot) clearSession(chatID int64) {
	b.sessionStore.Delete(chatID)
}

func downloadFile(url string) ([]byte, string, error) {
	client := &http.Client{Timeout: telegramDownloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("telegram file endpoint error")
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(content) > maxImageBytes {
		return nil, "", errors.New("file is too large")
	}
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	return content, mimeType, nil
}

func humanError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(strings.TrimPrefix(err.Error(), "Get "))
	if text == "" {
		return "неизвестная ошибка"
	}
	return text
}

func inferExtension(mimeType string) string { return extensionByMIME(mimeType) }

func paginate(values []string, page int, pageSize int) ([]string, bool, bool, int) {
	if pageSize <= 0 {
		pageSize = listPageSize
	}
	total := len(values)
	if total == 0 {
		return nil, false, false, 0
	}
	maxPage := (total - 1) / pageSize
	if page < 0 {
		page = 0
	}
	if page > maxPage {
		page = maxPage
	}
	start := page * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	return values[start:end], page > 0, page < maxPage, page
}

func stepPage(page int, dir string) int {
	switch dir {
	case "prev":
		if page > 0 {
			return page - 1
		}
		return 0
	case "next":
		return page + 1
	default:
		return page
	}
}

func parsePositiveInt(v string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0, errors.New("invalid int")
	}
	return n, nil
}

func filterOptions(values []string, enabled bool, query string) []string {
	if !enabled {
		return values
	}
	query = strings.TrimSpace(query)
	nq := normalizeLookup(query)
	if nq == "" {
		return values
	}
	type scored struct {
		value string
		score float64
	}
	out := make([]scored, 0, len(values))
	for _, v := range values {
		nv := normalizeLookup(v)
		if nv == "" {
			continue
		}
		score := fuzzyScore(nv, nq)
		if strings.Contains(nv, nq) {
			// Strong signal for search queries.
			if score < 0.92 {
				score = 0.92
			}
		}
		if score >= 0.45 {
			out = append(out, scored{value: v, score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score == out[j].score {
			return strings.ToLower(out[i].value) < strings.ToLower(out[j].value)
		}
		return out[i].score > out[j].score
	})
	flat := make([]string, 0, len(out))
	for _, s := range out {
		flat = append(flat, s.value)
	}
	return flat
}

func trimButtonLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	runes := []rune(v)
	if len(runes) <= maxButtonLabelRunes {
		return v
	}
	return string(runes[:maxButtonLabelRunes-1]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (b *Bot) callbackForValue(state *sessionState, field string, value string) string {
	data := fmt.Sprintf("set|%s|%s", field, value)
	if len(data) <= 64 {
		return data
	}
	if state == nil {
		return "noop|long|x"
	}
	if state.ValueMap == nil {
		state.ValueMap = map[string]string{}
	}
	token := fmt.Sprintf("%x", crc32.ChecksumIEEE([]byte(field+"|"+value)))
	state.ValueMap[token] = field + "|" + value
	return "pick|" + token + "|set"
}

func buildFileName(base string, mimeType string) string {
	base = strings.TrimSpace(filepath.Base(base))
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	if base == "" {
		base = fmt.Sprintf("img_%d", time.Now().Unix())
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" || !isAllowedImageExtension(base) {
		base = strings.TrimSuffix(base, filepath.Ext(base))
		base += inferExtension(mimeType)
	}
	return base
}

func (b *Bot) sendWithRetry(chattable tgbotapi.Chattable) error {
	observability.TelegramSend()
	var lastErr error
	for attempt := 0; attempt < telegramSendRetries; attempt++ {
		_, err := b.api.Send(chattable)
		if err == nil {
			return nil
		}
		if isMessageNotModifiedError(err) {
			return nil
		}
		lastErr = err
		if attempt > 0 {
			observability.TelegramRetry()
		}
		log.Printf("telegram send retryable=%v attempt=%d err=%v", isTransientTelegramError(err), attempt+1, err)
		if !isTransientTelegramError(err) || attempt == telegramSendRetries-1 {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
	}
	return lastErr
}

func isMessageNotModifiedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

func isTransientTelegramError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unexpected eof") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection aborted")
}
