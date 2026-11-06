package nacos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/allinshit/nacos-ops/internal/config"
)

const configAPIPath = "/nacos/v1/cs/configs"

type HTTPProvider struct {
	baseURL string
	client  *http.Client

	username string
	password string

	tokenMu        sync.Mutex
	accessToken    string
	tokenExpiresAt time.Time
}

type loginResponse struct {
	AccessToken string `json:"accessToken"`
	TokenTTL    int64  `json:"tokenTtl"`
}

func (p *HTTPProvider) getAccessToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if p.accessToken != "" && time.Now().Before(p.tokenExpiresAt) {
		return p.accessToken, nil
	}

	params := url.Values{}
	params.Set("username", p.username)
	params.Set("password", p.password)

	endpoint := p.baseURL + "/nacos/v1/auth/login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", fmt.Errorf("create nacos login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send nacos login request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read nacos login response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf(
			"nacos login failed: status=%d, body=%s",
			resp.StatusCode,
			strings.TrimSpace(string(respBody)),
		)
	}

	var result loginResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode nacos login response: %w", err)
	}

	if result.AccessToken == "" {
		return "", errors.New("nacos login response has empty accessToken")
	}

	if result.TokenTTL <= 0 {
		return "", fmt.Errorf("invalid nacos token TTL: %d", result.TokenTTL)
	}

	p.accessToken = result.AccessToken

	validDuration := time.Duration(result.TokenTTL) * time.Second * 9 / 10
	p.tokenExpiresAt = time.Now().Add(validDuration)

	return p.accessToken, nil
}

func NewHTTPProvider(serverAddr string, client *http.Client) (*HTTPProvider, error) {
	return newHTTPProvider(serverAddr, "", "", client)
}

func NewHTTPProviderWithAuth(
	serverAddr string,
	username string,
	password string,
	client *http.Client,
) (*HTTPProvider, error) {
	return newHTTPProvider(serverAddr, username, password, client)
}

func newHTTPProvider(
	serverAddr string,
	username string,
	password string,
	client *http.Client,
) (*HTTPProvider, error) {
	u, err := url.Parse(strings.TrimRight(serverAddr, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid nacos server address: %w", err)
	}

	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("nacos server address must be an absolute HTTP or HTTPS URL")
	}

	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("nacos server address must not contain query or fragment")
	}

	if client == nil {
		client = http.DefaultClient
	}

	return &HTTPProvider{
		baseURL: strings.TrimRight(u.String(), "/"),
		client:  client,

		username: username,
		password: password,
	}, nil
}

func validateNacosID(id config.ConfigID) error {
	if id.Group == "" {
		return errors.New("nacos config group is required")
	}
	return nil
}

func (p *HTTPProvider) Pull(ctx context.Context, id config.ConfigID) (*config.Config, error) {

	if err := validateNacosID(id); err != nil {
		return nil, err
	}

	params := configIDParams(id)

	body, status, err := p.do(ctx, http.MethodGet, params)
	if err != nil {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("%w: dataId=%s, group=%s",
				config.ErrConfigNotFound, id.DataID, id.Group)
		}
		return nil, err
	}

	return &config.Config{
		ID:      id,
		Content: string(body),
	}, nil
}

func (p *HTTPProvider) Publish(ctx context.Context, cfg config.Config) error {

	if err := validateNacosID(cfg.ID); err != nil {
		return err
	}

	params := configIDParams(cfg.ID)
	params.Set("content", cfg.Content)

	body, _, err := p.do(ctx, http.MethodPost, params)
	if err != nil {
		return err
	}

	return checkBooleanResponse(body)
}

func (p *HTTPProvider) Delete(ctx context.Context, id config.ConfigID) error {

	if err := validateNacosID(id); err != nil {
		return err
	}

	body, _, err := p.do(ctx, http.MethodDelete, configIDParams(id))
	if err != nil {
		return err
	}

	return checkBooleanResponse(body)
}

func configIDParams(id config.ConfigID) url.Values {
	params := url.Values{}
	params.Set("dataId", id.DataID)
	params.Set("group", id.Group)

	if id.Namespace != "" {
		params.Set("tenant", id.Namespace)
	}

	return params
}

func (p *HTTPProvider) do(
	ctx context.Context,
	method string,
	params url.Values,
) ([]byte, int, error) {

	if p.username != "" {
		token, err := p.getAccessToken(ctx)
		if err != nil {
			return nil, 0, err
		}

		params.Set("accessToken", token)
	}

	endpoint := p.baseURL + configAPIPath

	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(params.Encode())
	} else {
		endpoint += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, 0, fmt.Errorf("create nacos request: %w", err)
	}

	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("send nacos request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read nacos response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, resp.StatusCode, fmt.Errorf(
			"nacos request failed: status=%d, body=%s",
			resp.StatusCode,
			strings.TrimSpace(string(respBody)),
		)
	}

	return respBody, resp.StatusCode, nil
}

func checkBooleanResponse(body []byte) error {
	result, err := strconv.ParseBool(strings.TrimSpace(string(body)))
	if err != nil {
		return fmt.Errorf("invalid nacos response %q: %w", body, err)
	}

	if !result {
		return errors.New("nacos operation was rejected")
	}

	return nil
}
