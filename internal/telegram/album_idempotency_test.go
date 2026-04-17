package telegram

import "testing"

func TestAlbumFlushingMarkIdempotency(t *testing.T) {
	b := &Bot{flushing: map[string]struct{}{}}
	key := "1:group"

	if ok := b.tryMarkAlbumFlushing(key); !ok {
		t.Fatalf("expected first mark to succeed")
	}
	if ok := b.tryMarkAlbumFlushing(key); ok {
		t.Fatalf("expected second mark to be rejected")
	}
	b.unmarkAlbumFlushing(key)
	if ok := b.tryMarkAlbumFlushing(key); !ok {
		t.Fatalf("expected mark to succeed after unmark")
	}
}
