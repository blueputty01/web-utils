package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type Artifact struct {
	Key         string
	ContentType string
	Size        int64
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) (Artifact, error)
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	CleanupExpired(ctx context.Context, now time.Time) (int, error)
	Healthy(ctx context.Context) error
}

// VerifyWAV checks that data begins with a valid RIFF/WAVE header and has an allowed audio content type.
func VerifyWAV(data []byte) error {
	if len(data) < 44 {
		return fmt.Errorf("invalid WAV artifact: file too small (%d bytes)", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return fmt.Errorf("invalid WAV artifact: missing RIFF/WAVE signature")
	}
	detected := http.DetectContentType(data)
	if !strings.HasPrefix(detected, "audio/") && detected != "application/octet-stream" {
		return fmt.Errorf("unsupported artifact content type: %s", detected)
	}
	return nil
}

type MemoryStore struct {
	mu      sync.Mutex
	ttl     time.Duration
	objects map[string]memoryObject
}

type memoryObject struct {
	data     []byte
	artifact Artifact
}

func NewMemoryStore(ttl time.Duration) *MemoryStore {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &MemoryStore{
		ttl:     ttl,
		objects: make(map[string]memoryObject),
	}
}

func (m *MemoryStore) Put(_ context.Context, key string, data []byte, contentType string) (Artifact, error) {
	if strings.TrimSpace(key) == "" {
		return Artifact{}, fmt.Errorf("artifact key is required")
	}
	if err := VerifyWAV(data); err != nil {
		return Artifact{}, err
	}
	if contentType == "" {
		contentType = "audio/wav"
	}
	now := time.Now().UTC()
	art := Artifact{
		Key:         key,
		ContentType: contentType,
		Size:        int64(len(data)),
		CreatedAt:   now,
		ExpiresAt:   now.Add(m.ttl),
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	m.mu.Lock()
	m.objects[key] = memoryObject{data: copied, artifact: art}
	m.mu.Unlock()
	return art, nil
}

func (m *MemoryStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	obj, ok := m.objects[key]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("artifact not found: %s", key)
	}
	out := make([]byte, len(obj.data))
	copy(out, obj.data)
	return out, nil
}

func (m *MemoryStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.objects, key)
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) CleanupExpired(_ context.Context, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for k, obj := range m.objects {
		if !now.Before(obj.artifact.ExpiresAt) {
			delete(m.objects, k)
			removed++
		}
	}
	return removed, nil
}

func (m *MemoryStore) Healthy(_ context.Context) error {
	return nil
}

// S3Store implements Store against an S3-compatible object store (such as MinIO) using AWS Signature V4.
type S3Store struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	TTL       time.Duration
	HTTP      *http.Client

	mu          sync.Mutex
	bucketReady bool
	tracked     map[string]time.Time
}

func NewS3Store(endpoint, bucket, accessKey, secretKey string, ttl time.Duration) *S3Store {
	if bucket == "" {
		bucket = "artifacts"
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &S3Store{
		Endpoint:  strings.TrimRight(endpoint, "/"),
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Region:    "us-east-1",
		TTL:       ttl,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		tracked:   make(map[string]time.Time),
	}
}

func (s *S3Store) ensureBucket(ctx context.Context) error {
	s.mu.Lock()
	ready := s.bucketReady
	s.mu.Unlock()
	if ready {
		return nil
	}
	headResp, err := s.doSigned(ctx, http.MethodHead, "/"+s.Bucket, "", nil, "")
	if err == nil {
		headResp.Body.Close()
		if headResp.StatusCode == http.StatusOK {
			s.mu.Lock()
			s.bucketReady = true
			s.mu.Unlock()
			return nil
		}
	}
	putResp, err := s.doSigned(ctx, http.MethodPut, "/"+s.Bucket, "", nil, "")
	if err != nil {
		return fmt.Errorf("create bucket %s: %w", s.Bucket, err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("create bucket %s returned %d: %s", s.Bucket, putResp.StatusCode, strings.TrimSpace(string(body)))
	}
	s.mu.Lock()
	s.bucketReady = true
	s.mu.Unlock()
	return nil
}

func (s *S3Store) Put(ctx context.Context, key string, data []byte, contentType string) (Artifact, error) {
	if strings.TrimSpace(key) == "" {
		return Artifact{}, fmt.Errorf("artifact key is required")
	}
	if err := VerifyWAV(data); err != nil {
		return Artifact{}, err
	}
	if err := s.ensureBucket(ctx); err != nil {
		return Artifact{}, err
	}
	if contentType == "" {
		contentType = "audio/wav"
	}
	cleanKey := strings.TrimLeft(key, "/")
	resp, err := s.doSigned(ctx, http.MethodPut, "/"+s.Bucket+"/"+cleanKey, "", data, contentType)
	if err != nil {
		return Artifact{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return Artifact{}, fmt.Errorf("put object %s returned %d: %s", cleanKey, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	now := time.Now().UTC()
	art := Artifact{
		Key:         cleanKey,
		ContentType: contentType,
		Size:        int64(len(data)),
		CreatedAt:   now,
		ExpiresAt:   now.Add(s.TTL),
	}
	s.mu.Lock()
	s.tracked[cleanKey] = art.ExpiresAt
	s.mu.Unlock()
	return art, nil
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, error) {
	cleanKey := strings.TrimLeft(key, "/")
	resp, err := s.doSigned(ctx, http.MethodGet, "/"+s.Bucket+"/"+cleanKey, "", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("get object %s returned %d", cleanKey, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	cleanKey := strings.TrimLeft(key, "/")
	resp, err := s.doSigned(ctx, http.MethodDelete, "/"+s.Bucket+"/"+cleanKey, "", nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	s.mu.Lock()
	delete(s.tracked, cleanKey)
	s.mu.Unlock()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete object %s returned %d", cleanKey, resp.StatusCode)
	}
	return nil
}

type listBucketResult struct {
	Contents []struct {
		Key          string    `xml:"Key"`
		LastModified time.Time `xml:"LastModified"`
	} `xml:"Contents"`
}

func (s *S3Store) CleanupExpired(ctx context.Context, now time.Time) (int, error) {
	removed := 0
	s.mu.Lock()
	var expiredKeys []string
	for k, exp := range s.tracked {
		if !now.Before(exp) {
			expiredKeys = append(expiredKeys, k)
		}
	}
	s.mu.Unlock()

	for _, k := range expiredKeys {
		if err := s.Delete(ctx, k); err == nil {
			removed++
		}
	}

	// Also inspect bucket objects in case of worker restart.
	if err := s.ensureBucket(ctx); err != nil {
		return removed, nil
	}
	resp, err := s.doSigned(ctx, http.MethodGet, "/"+s.Bucket, "list-type=2", nil, "")
	if err != nil {
		return removed, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return removed, nil
	}
	var listed listBucketResult
	if err := xml.NewDecoder(resp.Body).Decode(&listed); err != nil {
		return removed, nil
	}
	for _, item := range listed.Contents {
		if !item.LastModified.IsZero() && !now.Before(item.LastModified.Add(s.TTL)) {
			if err := s.Delete(ctx, item.Key); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *S3Store) Healthy(ctx context.Context) error {
	if s.Endpoint == "" {
		return nil
	}
	return s.ensureBucket(ctx)
}

func (s *S3Store) doSigned(ctx context.Context, method, uriPath, rawQuery string, body []byte, contentType string) (*http.Response, error) {
	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = uriPath
	u.RawQuery = rawQuery

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	payloadHash := sha256Hex(body)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Host = u.Host
	req.Header.Set("Host", u.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	headersToSign := map[string]string{
		"host":                 u.Host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	if contentType != "" {
		headersToSign["content-type"] = contentType
	}
	var signedHeaderNames []string
	for k := range headersToSign {
		signedHeaderNames = append(signedHeaderNames, k)
	}
	sort.Strings(signedHeaderNames)

	var canonicalHeaders strings.Builder
	for _, k := range signedHeaderNames {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.TrimSpace(headersToSign[k]))
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(signedHeaderNames, ";")

	canonicalRequest := strings.Join([]string{
		method,
		uriPath,
		canonicalQueryString(rawQuery),
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.Region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(s.SecretKey, dateStamp, s.Region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.AccessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)

	return s.HTTP.Do(req)
}

func canonicalQueryString(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	var keys []string
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vList := vals[k]
		sort.Strings(vList)
		for _, v := range vList {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}
