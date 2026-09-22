package transfers

import (
	"context"
	"testing"
)

type noOpPublisher struct{}
func (noOpPublisher) Publish(string, any) {}

const checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestTransferLifecycle(t *testing.T) {
	s := NewService(NewMemoryRepository(), noOpPublisher{}, "http://localhost:8080")
	created, err := s.Create(context.Background(), CreateInput{
		SourceDeviceID: "ios-1", DestinationDeviceID: "android-1",
		Kind: KindPhoto, SizeBytes: 12, SHA256: checksum,
	})
	if err != nil { t.Fatal(err) }
	if created.Status != StatusUploading { t.Fatalf("expected uploading, got %s", created.Status) }

	ready, err := s.MarkUploaded(context.Background(), created.ID)
	if err != nil { t.Fatal(err) }
	if ready.Status != StatusReady { t.Fatalf("expected ready, got %s", ready.Status) }

	completed, err := s.Complete(context.Background(), created.ID)
	if err != nil { t.Fatal(err) }
	if completed.Status != StatusCompleted { t.Fatalf("expected completed, got %s", completed.Status) }
}

func TestCannotCompleteBeforeUpload(t *testing.T) {
	s := NewService(NewMemoryRepository(), noOpPublisher{}, "http://localhost:8080")
	created, err := s.Create(context.Background(), CreateInput{
		SourceDeviceID: "ios-1", DestinationDeviceID: "android-1",
		Kind: KindFile, SizeBytes: 5, SHA256: checksum,
	})
	if err != nil { t.Fatal(err) }
	if _, err := s.Complete(context.Background(), created.ID); err != ErrInvalidTransition {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}
