package transfers

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

type publishedEvent struct {
	deviceIDs []string
	eventType string
	payload   any
}

type noOpPublisher struct {
	events []publishedEvent
}

func (p *noOpPublisher) PublishToDevices(
	deviceIDs []string,
	eventType string,
	payload any,
) {
	p.events = append(p.events, publishedEvent{
		deviceIDs: append([]string(nil), deviceIDs...),
		eventType: eventType,
		payload:   payload,
	})
}

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
	publisher := &noOpPublisher{}
	s := NewService(NewMemoryRepository(), publisher, urls)

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
	if len(publisher.events) != 1 {
		t.Fatalf("expected one ready event, got %d", len(publisher.events))
	}
	readyEvent := publisher.events[0]
	if readyEvent.eventType != "transfer.ready" {
		t.Fatalf("unexpected event type %q", readyEvent.eventType)
	}
	if len(readyEvent.deviceIDs) != 1 ||
		readyEvent.deviceIDs[0] != "android-1" {
		t.Fatalf("ready event must target destination, got %#v", readyEvent.deviceIDs)
	}
	encoded, err := json.Marshal(readyEvent.payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" {
		t.Fatal("expected realtime payload")
	}
	var eventPayload map[string]any
	if err := json.Unmarshal(encoded, &eventPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := eventPayload["downloadUrl"]; ok {
		t.Fatal("realtime payload must not contain signed download URL")
	}
	if _, ok := eventPayload["uploadUrl"]; ok {
		t.Fatal("realtime payload must not contain signed upload URL")
	}

	completed, err := s.Complete(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", completed.Status)
	}
	if len(publisher.events) != 2 {
		t.Fatalf("expected completed event, got %d events", len(publisher.events))
	}
	completedEvent := publisher.events[1]
	if completedEvent.eventType != "transfer.completed" {
		t.Fatalf("unexpected completed event %q", completedEvent.eventType)
	}
	if len(completedEvent.deviceIDs) != 1 ||
		completedEvent.deviceIDs[0] != "ios-1" {
		t.Fatalf(
			"completed event must target source, got %#v",
			completedEvent.deviceIDs,
		)
	}
}

func TestCannotMarkReadyWithoutUploadedObject(t *testing.T) {
	s := NewService(
		NewMemoryRepository(),
		&noOpPublisher{},
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
	s := NewService(NewMemoryRepository(), &noOpPublisher{}, urls)
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
		&noOpPublisher{},
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


func TestCreateEnforcesContractLimits(t *testing.T) {
	tests := []struct {
		name string
		in   CreateInput
	}{
		{
			name: "payload larger than 1 GiB",
			in: CreateInput{
				SourceDeviceID:      "ios-1",
				DestinationDeviceID: "android-1",
				Kind:                KindFile,
				SizeBytes:           (1 << 30) + 1,
				SHA256:              checksum,
			},
		},
		{
			name: "display name longer than 255 characters",
			in: CreateInput{
				SourceDeviceID:      "ios-1",
				DestinationDeviceID: "android-1",
				Kind:                KindFile,
				DisplayName:         strings.Repeat("a", 256),
				SizeBytes:           1,
				SHA256:              checksum,
			},
		},
		{
			name: "content type longer than 120 characters",
			in: CreateInput{
				SourceDeviceID:      "ios-1",
				DestinationDeviceID: "android-1",
				Kind:                KindFile,
				ContentType:         strings.Repeat("a", 121),
				SizeBytes:           1,
				SHA256:              checksum,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(
				NewMemoryRepository(),
				&noOpPublisher{},
				fakeURLs{},
			)
			if _, err := service.Create(
				context.Background(),
				test.in,
			); err != ErrInvalidInput {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}

func TestCreateCountsUnicodeDisplayNameCharacters(t *testing.T) {
	service := NewService(
		NewMemoryRepository(),
		&noOpPublisher{},
		fakeURLs{},
	)

	_, err := service.Create(context.Background(), CreateInput{
		SourceDeviceID:      "ios-1",
		DestinationDeviceID: "android-1",
		Kind:                KindText,
		DisplayName:         strings.Repeat("é", 255),
		ContentType:         "text/plain",
		SizeBytes:           1,
		SHA256:              checksum,
	})
	if err != nil {
		t.Fatal(err)
	}
}


func TestCreateRejectsMalformedChecksum(t *testing.T) {
	service := NewService(
		NewMemoryRepository(),
		&noOpPublisher{},
		fakeURLs{},
	)

	for _, value := range []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("g", 64),
		checksum + "suffix",
	} {
		_, err := service.Create(context.Background(), CreateInput{
			SourceDeviceID:      "ios-1",
			DestinationDeviceID: "android-1",
			Kind:                KindFile,
			SizeBytes:           1,
			SHA256:              value,
		})
		if err != ErrInvalidInput {
			t.Fatalf("checksum %q: expected invalid input, got %v", value, err)
		}
	}
}
