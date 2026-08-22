package api

import (
	"io"
	"time"
)

const volumeTransferHTTPIdleTimeout = 15 * time.Minute

type volumeTransferDeadlineController interface {
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type volumeTransferDeadlineReader struct {
	reader     io.Reader
	controller volumeTransferDeadlineController
	timeout    time.Duration
}

func (reader *volumeTransferDeadlineReader) Read(buffer []byte) (int, error) {
	if reader.controller != nil {
		_ = reader.controller.SetReadDeadline(time.Now().Add(reader.timeout))
	}
	return reader.reader.Read(buffer)
}

func (reader *volumeTransferDeadlineReader) Close() error {
	if closer, ok := reader.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type volumeTransferDeadlineWriter struct {
	writer     io.Writer
	controller volumeTransferDeadlineController
	timeout    time.Duration
}

func (writer *volumeTransferDeadlineWriter) Write(buffer []byte) (int, error) {
	if writer.controller != nil {
		_ = writer.controller.SetWriteDeadline(time.Now().Add(writer.timeout))
	}
	return writer.writer.Write(buffer)
}
