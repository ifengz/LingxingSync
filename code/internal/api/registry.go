package api

import "lingxing-sync/internal/config"

// ClientRegistry keeps one Client and TokenHolder per configured account.
// Workers, account checks, and settings status must use this registry so a
// successful token refresh is visible to every consumer of that account.
type ClientRegistry struct {
	clients map[string]*Client
}

func NewClientRegistry(accounts []config.Account, baseURL string) *ClientRegistry {
	clients := make(map[string]*Client, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		clients[account.ID] = NewClient(account, baseURL)
	}
	return &ClientRegistry{clients: clients}
}

func (r *ClientRegistry) Get(accountID string) *Client {
	if r == nil {
		return nil
	}
	return r.clients[accountID]
}
