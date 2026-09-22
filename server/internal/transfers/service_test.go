package transfers

import (
	"context"
	"io/fs"
	"testing"
)

type noOpPublisher struct{}

func (noOpPublisher) Publish(string, any) {}

type fakeURLs struct {
	uploaded bool
	size     int64
	checksum string
}

func (f fakeURLs) UploadURL(
	_ context.Context,
	id string,
	_ string,
) (string, error) {
	return "https://upload.invalid/" + id, nil
}

func (f fakeURLs) DownloadURL(
	_ context.Context,
	id string,
) (string, error) {
	return "https://download.invalid/" + id, nil
}

func (f fakeURLs) InspectUploaded(
	_ context.Context,
	_ string,
) (int64, string, error) {
	if !f.uploaded {
		return 0, "", fs.ErrNotExist
	}
	return f.size, f.checksum, nil
}

const checksum = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestTransferLifecycle(t *testing.T) {
	urls := fakeURLs{uploaded: true, size: 12, checksum: checksum}
	s := NewService(NewMemoryRepository(), noOpPublisher{}, urls)

	created, err := s.Create(context.Background(), CreateInput{
		SourceDeviceID: "ios-1",
		DestinationDeviceID: "android-1",
		Kind: KindPhoto,
		SizeBytes: 12,
		SHA256: checksum,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusUploading {
		t.Fatalf("expected uploading, got %s", created.Status)
	}
	if created.UploadURL == "" {
		t.Fatal("expected signed upload URL")
	}

	ready, err := s.MarkUploaded(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != StatusReady {
		t.Fatalf("expected ready, got %s", ready.Status)
	}
	if ready.DownloadURL == "" {
		t.Fatal("expected signed download URL")
	}

	completed, err := s.Complete(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", completed.Status)
	}
}

func TestCannotMarkReadyWithoutUploadedObject(t *testing.T) {
	s := NewService(
		NewMemoryRepository(),
		noOpPublisher{},
		fakeURLs{},
	)
	created, err := s.Create(context.Background(), CreateInput{
		SourceDeviceID: "ios-1",
		DestinationDeviceID: "android-1",
		Kind: KindFile,
		SizeBytes: 5,
		SHA256: checksum,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.MarkUploaded(
		context.Background(),
		created.ID,
	); err != ErrUploadMissing {
		t.Fatalf("expected upload missing, got %v", err)
	}
}

func TestCannotMarkReadyWithWrongUploadedIntegrity(t *testing.T) {
	urls := fakeURLs{
		uploaded: true,
		size:     5,
		checksum: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	s := NewService(NewMemoryRepository(), noOpPublisher{}, urls)
	created, err := s.Create(context.Background(), CreateInput{
		SourceDeviceID: "ios-1",
		DestinationDeviceID: "android-1",
		Kind: KindFile,
		SizeBytes: 5,
		SHA256: checksum,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.MarkUploaded(
		context.Background(),
		created.ID,
	); err != ErrUploadIntegrity {
		t.Fatalf("expected upload integrity error, got %v", err)
	}
}

func TestCannotCompleteBeforeUpload(t *testing.T) {
	s := NewService(
		NewMemoryRepository(),
		noOpPublisher{},
		fakeURLs{},
	)
	created, err := s.Create(context.Background(), CreateInput{
		SourceDeviceID: "ios-1",
		DestinationDeviceID: "android-1",
		Kind: KindFile,
		SizeBytes: 5,
		SHA256: checksum,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(
		context.Background(),
		created.ID,
	); err != ErrInvalidTransition {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}
