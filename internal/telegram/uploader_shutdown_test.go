package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"PicFolderBot/internal/parser"
	"PicFolderBot/internal/service"
)

type slowFlow struct {
	delay time.Duration
}

func (s *slowFlow) ParseCaption(caption string) parser.ParsedInput { return parser.ParsedInput{} }
func (s *slowFlow) RootDisplayName() string                        { return "root" }
func (s *slowFlow) ListProducts() ([]string, error)                { return nil, nil }
func (s *slowFlow) ListColors(product string) ([]string, error)    { return nil, nil }
func (s *slowFlow) ListSections(product, color string) ([]string, error) {
	return nil, nil
}
func (s *slowFlow) InvalidateProducts()                      {}
func (s *slowFlow) InvalidateColors(product string)          {}
func (s *slowFlow) InvalidateSections(product, color string) {}
func (s *slowFlow) UploadImage(payload service.UploadPayload) (string, error) {
	time.Sleep(s.delay)
	return "", nil
}
func (s *slowFlow) UploadImageAtLevel(level string, payload service.UploadPayload) (string, error) {
	time.Sleep(s.delay)
	return "ok", nil
}
func (s *slowFlow) CreateFolderAtLevel(level, product, color, section, newFolder string) (string, error) {
	return "", nil
}

func TestUploaderStopWithTimeout(t *testing.T) {
	u := newUploader(&slowFlow{delay: 150 * time.Millisecond}, 1, 8)
	_ = u.submit(service.LevelSection, service.UploadPayload{})

	err := u.stopWithTimeout(context.Background(), 20*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got: %v", err)
	}
}
