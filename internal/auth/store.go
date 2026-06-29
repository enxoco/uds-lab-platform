package auth

import "sync"

// Store tracks authenticated clients: clientID → GitHub username.
// All methods are safe for concurrent use.
type Store struct {
	mu    sync.RWMutex
	users map[string]string
}

func NewStore() *Store {
	return &Store{users: make(map[string]string)}
}

func (s *Store) Set(clientID, githubUser string) {
	s.mu.Lock()
	s.users[clientID] = githubUser
	s.mu.Unlock()
}

// Get returns the GitHub username for a client, or ("", false) if not authenticated.
func (s *Store) Get(clientID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[clientID]
	return u, ok
}

func (s *Store) Delete(clientID string) {
	s.mu.Lock()
	delete(s.users, clientID)
	s.mu.Unlock()
}

type Entry struct {
	ClientID   string
	GitHubUser string
}

func (s *Store) All() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.users))
	for cid, u := range s.users {
		out = append(out, Entry{ClientID: cid, GitHubUser: u})
	}
	return out
}
