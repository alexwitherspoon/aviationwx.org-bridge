package scheduler

import (
	"context"
)

// mockCamera is a mock camera for testing
type mockCamera struct {
	id      string
	camType string
	data    []byte
	err     error
}

func (m *mockCamera) Capture(ctx context.Context) ([]byte, error) {
	return m.data, m.err
}

func (m *mockCamera) ID() string {
	return m.id
}

func (m *mockCamera) Type() string {
	return m.camType
}

// mockUploader is a mock upload client for testing
type mockUploader struct {
	err error
}

func (m *mockUploader) Upload(remotePath string, data []byte) error {
	return m.err
}

func (m *mockUploader) TestConnection() error {
	return m.err
}

// panicUploader panics on Upload to test panic recovery
type panicUploader struct{}

func (p *panicUploader) Upload(remotePath string, data []byte) error {
	panic("panicUploader: intentional panic for testing")
}

func (p *panicUploader) TestConnection() error {
	return nil
}
