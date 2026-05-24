package hetzner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

func (c *Client) CreateServer(ctx context.Context, req CreateServerRequest) (id int64, ip string, err error) {
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
	defer resp.Body.Close()

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
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/servers/%d", apiBase, id), nil)
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("hcloud delete server %d: status %d", id, resp.StatusCode)
	}
	return nil
}

func (c *Client) setHeaders(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Content-Type", "application/json")
}
