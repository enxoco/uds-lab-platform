package hetzner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const apiBase = "https://api.hetzner.cloud/v1"

type Client struct {
	token string
	http  *http.Client
}

func New(token string) *Client {
	return &Client{token: token, http: &http.Client{}}
}

type CreateServerRequest struct {
	Name       string   `json:"name"`
	ServerType string   `json:"server_type"`
	Image      string   `json:"image"`
	Location   string   `json:"location"`
	UserData   string   `json:"user_data"`
	SSHKeys    []string `json:"ssh_keys,omitempty"`
}

type createServerResponse struct {
	Server struct {
		ID        int64 `json:"id"`
		PublicNet struct {
			IPv4 struct {
				IP string `json:"ip"`
			} `json:"ipv4"`
		} `json:"public_net"`
	} `json:"server"`
}

type imageResponse struct {
	Images []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"images"`
}

// FindLatestSnapshot returns the numeric ID of the most recently created
// snapshot matching the given Hetzner label selector.
// Example label selector:
//
//	role=uds-lab-playground,tier=uds-core
//	role=uds-lab-base
func (c *Client) FindLatestSnapshot(ctx context.Context, labelSelector string) (string, error) {
	q := url.Values{}
	q.Set("type", "snapshot")
	q.Set("sort", "created:desc")
	q.Set("per_page", "1")
	q.Set("label_selector", labelSelector)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/images?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}

	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hcloud list images: status %d", resp.StatusCode)
	}

	var out imageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}

	if len(out.Images) == 0 {
		return "", nil
	}

	return strconv.FormatInt(out.Images[0].ID, 10), nil
}

func (c *Client) resolveImageID(ctx context.Context, nameOrID string) (string, error) {
	if nameOrID == "" {
		return "", nil
	}

	// If it's already numeric, don't bother doing a name lookup.
	if _, err := strconv.ParseInt(nameOrID, 10, 64); err == nil {
		return nameOrID, nil
	}

	q := url.Values{}
	q.Set("name", nameOrID)
	q.Set("type", "snapshot")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/images?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}

	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hcloud resolve image %q: status %d", nameOrID, resp.StatusCode)
	}

	var out imageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}

	if len(out.Images) > 0 {
		return strconv.FormatInt(out.Images[0].ID, 10), nil
	}

	return nameOrID, nil
}

func (c *Client) CreateServer(ctx context.Context, req CreateServerRequest) (id int64, ip string, err error) {
	imageID, err := c.resolveImageID(ctx, req.Image)
	if err != nil {
		return 0, "", fmt.Errorf("resolve image: %w", err)
	}

	req.Image = imageID

	body, err := json.Marshal(req)
	if err != nil {
		return 0, "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/servers", bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}

	c.setHeaders(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return 0, "", fmt.Errorf("hcloud create server: status %d", resp.StatusCode)
	}

	var out createServerResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, "", err
	}

	return out.Server.ID, out.Server.PublicNet.IPv4.IP, nil
}

func (c *Client) DeleteServer(ctx context.Context, id int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/servers/%d", apiBase, id), nil)
	if err != nil {
		return err
	}

	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("hcloud delete server %d: status %d", id, resp.StatusCode)
	}

	return nil
}

func (c *Client) setHeaders(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
}
