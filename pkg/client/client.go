package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
	"launchdarkly/internal/store"
)

const defaultSyncInterval = 30 * time.Second

var ErrFlagNotFound = errors.New("flag not found")

type Mode string

const (
	ModeRemote Mode = "remote"
	ModeLocal  Mode = "local"
)

type Config struct {
	Mode         Mode
	BaseURL      string
	HTTPClient   *http.Client
	SyncInterval time.Duration
}

type Client struct {
	mode         Mode
	baseURL      string
	httpClient   *http.Client
	store        *store.Holder
	syncInterval time.Duration

	closeOnce sync.Once
	closeCh   chan struct{}
	doneCh    chan struct{}

	mu        sync.RWMutex
	lastSync  time.Time
	lastError error
}

type evaluateRequest struct {
	FlagKey string         `json:"flag_key"`
	Context domain.Context `json:"context"`
}

type evaluateResponse struct {
	Variant string `json:"variant"`
}

type apiErrorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(cfg Config) (*Client, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = ModeRemote
	}
	if mode != ModeRemote && mode != ModeLocal {
		return nil, fmt.Errorf("invalid mode %q", mode)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("base URL is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	c := &Client{
		mode:       mode,
		baseURL:    baseURL,
		httpClient: httpClient,
		store:      store.NewHolder(store.Empty()),
		closeCh:    make(chan struct{}),
		doneCh:     make(chan struct{}),
	}

	if mode != ModeLocal {
		close(c.doneCh)
		return c, nil
	}

	c.syncInterval = cfg.SyncInterval
	if c.syncInterval <= 0 {
		c.syncInterval = defaultSyncInterval
	}

	if err := c.Sync(context.Background()); err != nil {
		c.setLastError(err)
		close(c.doneCh)
		return nil, fmt.Errorf("initial local sync failed: %w", err)
	}

	go c.runSyncLoop()

	return c, nil
}

func (c *Client) Eval(flagKey string, ctx domain.Context) (string, error) {
	return c.EvalContext(context.Background(), flagKey, ctx)
}

func (c *Client) EvalContext(ctx context.Context, flagKey string, evalCtx domain.Context) (string, error) {
	flagKey = strings.TrimSpace(flagKey)
	if flagKey == "" {
		return "", errors.New("flag key is required")
	}

	if c.mode == ModeLocal {
		variant, found := c.store.Evaluate(flagKey, &evalCtx)
		if !found {
			return "", ErrFlagNotFound
		}
		return variant, nil
	}

	reqBody, err := json.Marshal(evaluateRequest{
		FlagKey: flagKey,
		Context: evalCtx,
	})
	if err != nil {
		return "", fmt.Errorf("marshal evaluate request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/evaluate", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build evaluate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("evaluate request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", decodeAPIError(resp)
	}

	var parsed evaluateResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode evaluate response: %w", err)
	}

	return parsed.Variant, nil
}

func (c *Client) Sync(ctx context.Context) error {
	if c.mode != ModeLocal {
		return errors.New("sync is only supported in local mode")
	}

	flags, err := c.fetchFlags(ctx)
	if err != nil {
		c.setLastError(err)
		return err
	}

	compiled := make([]*eval.CompiledFlag, 0, len(flags))
	for _, flag := range flags {
		compiledFlag, err := eval.CompileFlag(flag)
		if err != nil {
			slog.Warn("client local sync skipped invalid flag", "key", flag.Key, "error", err)
			continue
		}
		compiled = append(compiled, compiledFlag)
	}

	c.store.Swap(store.New(compiled...))
	c.mu.Lock()
	c.lastSync = time.Now().UTC()
	c.lastError = nil
	c.mu.Unlock()
	return nil
}

func (c *Client) Close() error {
	if c.mode != ModeLocal {
		return nil
	}

	c.closeOnce.Do(func() {
		close(c.closeCh)
		<-c.doneCh
	})

	return nil
}

func (c *Client) LastSync() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSync
}

func (c *Client) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

func (c *Client) runSyncLoop() {
	defer close(c.doneCh)

	ticker := time.NewTicker(c.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ticker.C:
			if err := c.Sync(context.Background()); err != nil {
				slog.Warn("client local sync failed; keeping previous store", "error", err)
			}
		}
	}
}

func (c *Client) fetchFlags(ctx context.Context) ([]domain.Flag, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/flags", nil)
	if err != nil {
		return nil, fmt.Errorf("build flags request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("flags request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}

	var flags []domain.Flag
	if err := json.NewDecoder(resp.Body).Decode(&flags); err != nil {
		return nil, fmt.Errorf("decode flags response: %w", err)
	}

	return flags, nil
}

func (c *Client) setLastError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err
}

func decodeAPIError(resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var envelope apiErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil {
		if envelope.Error.Code == "flag_not_found" {
			return ErrFlagNotFound
		}
		if envelope.Error.Message != "" {
			return fmt.Errorf("api error (%s): %s", envelope.Error.Code, envelope.Error.Message)
		}
	}

	return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
