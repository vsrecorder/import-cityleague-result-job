package simplemq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
)

type SimpleMQ interface {
	SendMessage(ctx context.Context, msgReq *SendMessageRequest) (*SendMessageResponse, error)
	ReceiveMessage(ctx context.Context) (*ReceiveMessageResponse, error)
	UpdateMessageTimeout(ctx context.Context, msgID string) error
	DeleteMessage(ctx context.Context, msgID string) error
}

type SendMessageRequest struct {
	Content string `json:"content"`
}

type SendMessageResponse struct {
	Result  string   `json:"result"`
	Message *Message `json:"message"`
}

type ReceiveMessageResponse struct {
	Result   string     `json:"result"`
	Messages []*Message `json:"messages"`
}

type Message struct {
	ID                  string    `json:"id"`
	Content             string    `json:"content"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	AcquiredAt          time.Time `json:"acquired_at"`
	VisibilityTimeoutAt time.Time `json:"visibility_timeout_at"`
}

func (m *Message) UnmarshalJSON(b []byte) error {
	msg := struct {
		ID                  string `json:"id"`
		Content             string `json:"content"`
		CreatedAt           int64  `json:"created_at"`
		UpdatedAt           int64  `json:"updated_at"`
		ExpiresAt           int64  `json:"expires_at"`
		AcquiredAt          int64  `json:"acquired_at"`
		VisibilityTimeoutAt int64  `json:"visibility_timeout_at"`
	}{}

	if err := json.Unmarshal(b, &msg); err != nil {
		return err
	}

	m.ID = msg.ID
	m.Content = msg.Content
	m.CreatedAt = time.UnixMilli(msg.CreatedAt)
	m.UpdatedAt = time.UnixMilli(msg.UpdatedAt)
	m.ExpiresAt = time.UnixMilli(msg.ExpiresAt)
	m.AcquiredAt = time.UnixMilli(msg.AcquiredAt)
	m.VisibilityTimeoutAt = time.UnixMilli(msg.VisibilityTimeoutAt)

	return nil
}

type SimpleMQClient struct {
	queueName  string
	token      string
	httpClient *http.Client
}

func NewSimpleMQClient(queueName, token string) SimpleMQ {
	return &SimpleMQClient{
		queueName:  queueName,
		token:      token,
		httpClient: http.DefaultClient,
	}
}

func (c *SimpleMQClient) setHeader(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+c.token)
}

func (c *SimpleMQClient) SendMessage(ctx context.Context, msgReq *SendMessageRequest) (*SendMessageResponse, error) {
	b := bytes.Buffer{}
	if err := json.NewEncoder(&b).Encode(msgReq); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://simplemq.tk1b.api.sacloud.jp/v1/queues/%s/messages", c.queueName),
		&b,
	)
	if err != nil {
		return nil, err
	}

	c.setHeader(req)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	var ret SendMessageResponse
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}

	return &ret, nil
}

func (c *SimpleMQClient) ReceiveMessage(ctx context.Context) (*ReceiveMessageResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://simplemq.tk1b.api.sacloud.jp/v1/queues/%s/messages", c.queueName),
		nil,
	)
	if err != nil {
		return nil, err
	}

	c.setHeader(req)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.New("fatal")
	}

	var ret ReceiveMessageResponse
	if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
		return nil, err
	}

	return &ret, nil
}

func (c *SimpleMQClient) UpdateMessageTimeout(ctx context.Context, msgID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("https://simplemq.tk1b.api.sacloud.jp/v1/queues/%s/messages/%s", c.queueName, msgID),
		nil,
	)
	if err != nil {
		return err
	}

	c.setHeader(req)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}

		return errors.New("fatal")
	}

	return nil
}

func (c *SimpleMQClient) DeleteMessage(ctx context.Context, msgID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("https://simplemq.tk1b.api.sacloud.jp/v1/queues/%s/messages/%s", c.queueName, msgID),
		nil,
	)
	if err != nil {
		return err
	}

	c.setHeader(req)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errors.New("fatal")
	}

	return nil
}
