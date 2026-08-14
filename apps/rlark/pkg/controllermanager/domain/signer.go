package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/rlinf/rlark/apps/rlark/pkg/server"
)

type signer struct {
	once   sync.Once
	r      *Reconciler
	client *server.Client
}

func newSigner(r *Reconciler) *signer {
	return &signer{
		r: r,
	}
}

func (s *signer) initClient(ctx context.Context) error {
	var err error
	s.once.Do(func() {
		s.client, err = server.NewClientFromKubernetes(ctx, s.r.ServerAddress, s.r.KubeClientConfig)
	})
	return err
}

// Sign signs the input.
func (s *signer) Sign(ctx context.Context, domainID, agentID string) ([]byte, []byte, error) {
	if err := s.initClient(ctx); err != nil {
		return nil, nil, err
	}
	if s.client == nil {
		return nil, nil, fmt.Errorf("server client is not initialized")
	}
	target := s.client.BuildURL("api", "sign")
	resp, err := s.client.DoRequestWithObject(ctx, http.MethodPost, target, server.SignRequest{
		Role:     "domain",
		ClientID: agentID,
		DomainID: domainID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("send sign request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("sign certificate failed with status: %s", resp.Status)
	}
	var respData server.SignResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return []byte(respData.CertPEM), []byte(respData.KeyPEM), nil
}
