package telegram

import (
	"testing"

	"PicFolderBot/internal/parser"
	"PicFolderBot/internal/service"
)

func TestUploaderSubmitAfterStopReturnsError(t *testing.T) {
	u := newUploader(&stubFlowAPI{}, 1, 1)
	u.stop()

	ch := u.submit(service.LevelProduct, service.UploadPayload{})
	res := <-ch
	if res.Err == nil {
		t.Fatalf("expected error when submitting to stopped uploader")
	}
}

type stubFlowAPI struct{}

func (s *stubFlowAPI) ParseCaption(caption string) parser.ParsedInput { return parser.ParsedInput{} }
func (s *stubFlowAPI) RootDisplayName() string                        { return "root" }
func (s *stubFlowAPI) ListProducts() ([]string, error) {
	return nil, nil
}
func (s *stubFlowAPI) ListColors(product string) ([]string, error) {
	return nil, nil
}
func (s *stubFlowAPI) ListSections(product, color string) ([]string, error) {
	return nil, nil
}
func (s *stubFlowAPI) InvalidateProducts()                      {}
func (s *stubFlowAPI) InvalidateColors(product string)          {}
func (s *stubFlowAPI) InvalidateSections(product, color string) {}
func (s *stubFlowAPI) UploadImage(payload service.UploadPayload) (string, error) {
	return "", nil
}
func (s *stubFlowAPI) UploadImageAtLevel(level string, payload service.UploadPayload) (string, error) {
	return "ok", nil
}
func (s *stubFlowAPI) CreateFolderAtLevel(level, product, color, section, newFolder string) (string, error) {
	return "", nil
}
