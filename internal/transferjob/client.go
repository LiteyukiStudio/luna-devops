package transferjob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const maxRemoteErrorBytes = 16 * 1024

type RemoteError struct {
	Status int
	Code   string
}

func (err *RemoteError) Error() string {
	if err == nil {
		return CodeCallbackUnavailable
	}
	if stableErrorCodePattern.MatchString(err.Code) {
		return err.Code
	}
	return CodeCallbackUnavailable
}

type ContentInfo struct {
	Offset    int64
	Size      int64
	ChunkSize int64
	ETag      string
}

type CompleteInput struct {
	ExpectedState    string `json:"expectedState"`
	TransferredBytes int64  `json:"transferredBytes"`
	SHA256           string `json:"sha256"`
	LogicalBytes     int64  `json:"logicalBytes"`
	DataSHA256       string `json:"dataSHA256"`
}

type ProgressInput struct {
	ExpectedState    string `json:"expectedState"`
	TransferredBytes int64  `json:"transferredBytes"`
	ProcessedFiles   int64  `json:"processedFiles"`
	Stage            string `json:"stage"`
}

type FailInput struct {
	ExpectedState string `json:"expectedState"`
	ErrorCode     string `json:"errorCode"`
	Diagnostic    string `json:"diagnostic"`
}

type Client struct {
	baseURL    *url.URL
	transferID string
	token      []byte
	http       *http.Client
	closeOnce  sync.Once
}

func NewClient(config Config, httpClient *http.Client) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(config.CallbackBaseURL)
	if err != nil {
		return nil, invalidConfig("callback URL")
	}
	token, err := readTokenFile(config.TokenFile)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Transport: http.DefaultTransport}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: baseURL, transferID: config.TransferID, token: token, http: &clientCopy}, nil
}

func (client *Client) Close() error {
	if client == nil {
		return nil
	}
	client.closeOnce.Do(func() {
		for index := range client.token {
			client.token[index] = 0
		}
	})
	return nil
}

func (client *Client) HeadContent(ctx context.Context) (ContentInfo, error) {
	response, end, err := client.execute(ctx, http.MethodHead, "content", nil, nil)
	if err != nil {
		return ContentInfo{}, err
	}
	defer response.Body.Close()
	if err := remoteResponseError(response); err != nil {
		end(err)
		return ContentInfo{}, err
	}
	offset, err := nonNegativeHeader(response.Header, "Upload-Offset")
	if err != nil {
		end(err)
		return ContentInfo{}, err
	}
	size, err := nonNegativeHeader(response.Header, "Upload-Length")
	if err != nil {
		end(err)
		return ContentInfo{}, err
	}
	chunkSize, err := positiveHeader(response.Header, "Upload-Chunk-Size")
	if err != nil || chunkSize < minimumChunkSize || chunkSize > maximumChunkSize || chunkSize%(1024*1024) != 0 {
		if err == nil {
			err = newError(CodeStateConflict, nil)
		}
		end(err)
		return ContentInfo{}, err
	}
	end(nil)
	return ContentInfo{Offset: offset, Size: size, ChunkSize: chunkSize, ETag: strings.TrimSpace(response.Header.Get("ETag"))}, nil
}

func (client *Client) OpenContent(ctx context.Context, offset int64) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, newError(CodeStateConflict, nil)
	}
	headers := http.Header{}
	if offset > 0 {
		headers.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	response, end, err := client.execute(ctx, http.MethodGet, "content", headers, nil)
	if err != nil {
		return nil, err
	}
	if err := remoteResponseError(response); err != nil {
		response.Body.Close()
		end(err)
		return nil, err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		err := &RemoteError{Status: response.StatusCode, Code: CodeCallbackUnavailable}
		response.Body.Close()
		end(err)
		return nil, err
	}
	return &operationReadCloser{ReadCloser: response.Body, end: end}, nil
}

func (client *Client) WritePart(ctx context.Context, offset int64, content []byte) (int64, error) {
	if offset < 0 || len(content) < 1 || int64(len(content)) > maximumChunkSize {
		return 0, newError(CodeStateConflict, nil)
	}
	digest := sha256.Sum256(content)
	return client.WritePartStream(ctx, offset, bytes.NewReader(content), int64(len(content)), digest[:])
}

func (client *Client) WritePartStream(ctx context.Context, offset int64, content io.Reader, size int64, digest []byte) (int64, error) {
	if offset < 0 || content == nil || size < 1 || size > maximumChunkSize || len(digest) != sha256.Size {
		return 0, newError(CodeStateConflict, nil)
	}
	headers := http.Header{
		"Tus-Resumable":   []string{"1.0.0"},
		"Upload-Offset":   []string{strconv.FormatInt(offset, 10)},
		"Upload-Checksum": []string{"sha256 " + base64.StdEncoding.EncodeToString(digest)},
		"Content-Type":    []string{"application/offset+octet-stream"},
		"Content-Length":  []string{strconv.FormatInt(size, 10)},
	}
	response, end, err := client.execute(ctx, http.MethodPatch, "content", headers, content)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if err := remoteResponseError(response); err != nil {
		end(err)
		return 0, err
	}
	if response.StatusCode != http.StatusNoContent {
		err := &RemoteError{Status: response.StatusCode, Code: CodeCallbackUnavailable}
		end(err)
		return 0, err
	}
	next, err := nonNegativeHeader(response.Header, "Upload-Offset")
	if err == nil && next != offset+size {
		err = newError(CodeStateConflict, nil)
	}
	end(err)
	return next, err
}

func (client *Client) Progress(ctx context.Context, input ProgressInput) error {
	if input.ExpectedState != "running" || input.TransferredBytes < 0 || input.ProcessedFiles < 0 || !stableStage(input.Stage) {
		return newError(CodeStateConflict, nil)
	}
	return client.postJSON(ctx, "progress", input)
}

func (client *Client) Complete(ctx context.Context, input CompleteInput) error {
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	input.DataSHA256 = strings.ToLower(strings.TrimSpace(input.DataSHA256))
	if input.ExpectedState != "running" || input.TransferredBytes < 0 || !isSHA256(input.SHA256) ||
		input.LogicalBytes < 0 || (input.LogicalBytes == 0) != (input.DataSHA256 == "") ||
		(input.DataSHA256 != "" && !isSHA256(input.DataSHA256)) {
		return newError(CodeChecksumMismatch, nil)
	}
	return client.postJSON(ctx, "complete", input)
}

func (client *Client) Fail(ctx context.Context, input FailInput) error {
	if input.ExpectedState != "running" {
		return newError(CodeStateConflict, nil)
	}
	input.ErrorCode = sanitizeStableCode(input.ErrorCode)
	if !stableDiagnostic(input.Diagnostic) {
		input.Diagnostic = "volume transfer job failed"
	}
	return client.postJSON(ctx, "fail", input)
}

func (client *Client) postJSON(ctx context.Context, action string, input any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return newError(CodeJobFailed, err)
	}
	headers := http.Header{"Content-Type": []string{"application/json"}}
	response, end, err := client.execute(ctx, http.MethodPost, action, headers, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := remoteResponseError(response); err != nil {
		end(err)
		return err
	}
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		err := &RemoteError{Status: response.StatusCode, Code: CodeCallbackUnavailable}
		end(err)
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	end(nil)
	return nil
}

func (client *Client) execute(ctx context.Context, method, action string, headers http.Header, body io.Reader) (*http.Response, telemetry.OperationEnd, error) {
	if client == nil || client.http == nil || client.baseURL == nil || len(client.token) == 0 {
		return nil, nil, newError(CodeCallbackUnavailable, nil)
	}
	requestURL := *client.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/internal/v1/volume-transfers/" + url.PathEscape(client.transferID) + "/" + action
	requestURL.RawPath = ""
	operationCtx, end := telemetry.StartOperationWithKind(ctx, "volume", "transfer_callback", trace.SpanKindClient)
	request, err := http.NewRequestWithContext(operationCtx, method, requestURL.String(), body)
	if err != nil {
		end(err)
		return nil, nil, newError(CodeCallbackUnavailable, err)
	}
	for key, values := range headers {
		request.Header[key] = append([]string(nil), values...)
	}
	if rawLength := request.Header.Get("Content-Length"); rawLength != "" {
		length, parseErr := strconv.ParseInt(rawLength, 10, 64)
		if parseErr != nil || length < 0 {
			end(newError(CodeCallbackUnavailable, parseErr))
			return nil, nil, newError(CodeCallbackUnavailable, parseErr)
		}
		request.ContentLength = length
		request.Header.Del("Content-Length")
	}
	request.Header.Set("Authorization", "Bearer "+string(client.token))
	propagation.TraceContext{}.Inject(operationCtx, propagation.HeaderCarrier(request.Header))
	response, err := client.http.Do(request)
	if err != nil {
		safeErr := newError(CodeCallbackUnavailable, err)
		end(safeErr)
		return nil, nil, safeErr
	}
	return response, end, nil
}

func remoteResponseError(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	var payload struct {
		Code string `json:"code"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxRemoteErrorBytes))
	_ = decoder.Decode(&payload)
	code := strings.TrimSpace(payload.Code)
	if !stableTransferErrorCode(code) {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = CodeCallbackUnauthorized
		case http.StatusConflict, http.StatusGone:
			code = CodeStateConflict
		case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
			code = CodeStoreUnavailable
		default:
			code = CodeCallbackUnavailable
		}
	}
	return &RemoteError{Status: response.StatusCode, Code: code}
}

func readTokenFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, newError(CodeCallbackUnauthorized, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Size() < 32 || info.Size() > 512 {
		return nil, newError(CodeCallbackUnauthorized, err)
	}
	token, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || !validBearerTokenBytes(token) {
		for index := range token {
			token[index] = 0
		}
		return nil, newError(CodeCallbackUnauthorized, err)
	}
	return token, nil
}

func validBearerTokenBytes(token []byte) bool {
	if len(token) < 32 || len(token) > 512 {
		return false
	}
	padding := false
	for _, value := range token {
		if value == '=' {
			padding = true
			continue
		}
		if padding || !isToken68Byte(value) {
			return false
		}
	}
	return true
}

func isToken68Byte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' ||
		value == '-' || value == '.' || value == '_' || value == '~' || value == '+' || value == '/'
}

func nonNegativeHeader(headers http.Header, name string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(headers.Get(name)), 10, 64)
	if err != nil || value < 0 {
		return 0, newError(CodeCallbackUnavailable, err)
	}
	return value, nil
}

func positiveHeader(headers http.Header, name string) (int64, error) {
	value, err := nonNegativeHeader(headers, name)
	if err != nil || value < 1 {
		return 0, newError(CodeCallbackUnavailable, err)
	}
	return value, nil
}

func stableStage(value string) bool {
	switch value {
	case "starting", "downloading", "extracting", "reading_device", "archiving", "uploading", "verifying", "completed":
		return true
	default:
		return false
	}
}

func stableDiagnostic(value string) bool {
	return len(value) > 0 && len(value) <= 256 && !strings.ContainsAny(value, "\r\n/\\:")
}

type operationReadCloser struct {
	io.ReadCloser
	end     telemetry.OperationEnd
	once    sync.Once
	readErr error
}

func (reader *operationReadCloser) Read(content []byte) (int, error) {
	count, err := reader.ReadCloser.Read(content)
	if err != nil && err != io.EOF {
		reader.readErr = err
	}
	return count, err
}

func (reader *operationReadCloser) Close() error {
	closeErr := reader.ReadCloser.Close()
	operationErr := reader.readErr
	if operationErr == nil {
		operationErr = closeErr
	}
	reader.once.Do(func() { reader.end(operationErr) })
	return closeErr
}
