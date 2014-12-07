// simplistic http sessions based on memcache, with in-memory stub for development
package gomemssn

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"github.com/bradfitz/gomemcache/memcache"
	"log"
	"net/http"
	"sync"
	"time"
)

// NewManager returns a new *Manager with sensible defaults.
// You need to provide the memcache client and
// an optional prefix for the keys we store in memcache.
func NewManager(client *memcache.Client, keyPrefix string) *Manager {

	if client == nil {
		log.Printf("NOTE: Memcache client is nil, falling back to storing sessions in memory with no expiration! This should only occur in a development environment, not in production.")
	}

	return &Manager{
		Expiration:        time.Minute * 30,
		TemplateCookie:    &http.Cookie{Name: keyPrefix + "_gomemssn", Path: "/", MaxAge: 60 * 30},
		MemcacheKeyPrefix: keyPrefix,
		Client:            client,
		stubClient:        make(map[string]*Session),
	}

}

func newKey() string {
	b := make([]byte, 33)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

type Manager struct {
	TemplateCookie    *http.Cookie        // this cookie is copied and the value modified for each one written to the client
	Expiration        time.Duration       // how long until session expiration - passed back to memcache
	Client            *memcache.Client    // the memcache client or nil to mean store in memory (stub for development)
	MemcacheKeyPrefix string              // prefix memcache keys with this
	stubClient        map[string]*Session // if client is null then we store sessions in memory here
	stubClientMutex   sync.RWMutex        // control access to stubClient
}

type Session struct {
	Key    string       // the key for this session
	Cookie *http.Cookie // the cookie we will write to the client
	Values Values       // values of the session
}

type Values map[string]interface {
}

// TODO: make a way to delete a session and recreate it with a new id - to prevent
// session fixation attacks.  You would call this function after logging in or
// other access escalation, to avoid someone else piggy backing on your session.

// Get or create the session object, sets the appropriate cookie, does
// not write to the backing store
func (m *Manager) Session(w http.ResponseWriter, r *http.Request) (ret *Session, err error) {

	name := m.TemplateCookie.Name
	if name == "" {
		return nil, fmt.Errorf("TemplateCookie cannot have empty string as name - put something in there")
	}

	cookie, err := r.Cookie(name)
	if err == nil && len(cookie.Value) > 0 {

		key := cookie.Value

		if m.Client != nil {

			it, err := m.Client.Get(key)
			if err == memcache.ErrCacheMiss {
				ret = &Session{Key: key, Values: make(Values)}
			} else if err != nil {
				return nil, err
			} else {
				ret = &Session{Key: key, Values: make(Values)}
				err = gob.NewDecoder(bytes.NewReader(it.Value)).Decode(&ret.Values)
				if err != nil {
					return nil, err
				}
			}

		} else {
			// look up the stub session
			m.stubClientMutex.RLock()
			ret = m.stubClient[key]
			m.stubClientMutex.RUnlock()
			if ret == nil {
				ret = &Session{Key: newKey(), Values: make(Values)}
			}
		}

	} else {
		// new empty session
		ret = &Session{Key: newKey(), Values: make(Values)}
	}

	// copy the cookie
	newc := *m.TemplateCookie
	// newc.MaxAge = int(m.Expiration / time.Second)
	newc.Value = ret.Key
	ret.Cookie = &newc

	// set it on the response writer - so the key goes back to the client
	http.SetCookie(w, ret.Cookie)

	return ret, nil

}

func (m *Manager) MustSession(w http.ResponseWriter, r *http.Request) *Session {
	ret, err := m.Session(w, r)
	if err != nil {
		panic(err)
	}
	return ret
}

// write the actual session back to he memcache backend
func (m *Manager) WriteSession(w http.ResponseWriter, s *Session) error {

	key := s.Key

	if m.Client == nil {
		m.stubClientMutex.Lock()
		m.stubClient[key] = s
		m.stubClientMutex.Unlock()
	} else {

		buf := &bytes.Buffer{}
		enc := gob.NewEncoder(buf)
		err := enc.Encode(s.Values)
		if err != nil {
			return err
		}
		exp := int32(m.Expiration / time.Second)
		err = m.Client.Set(&memcache.Item{Key: key, Value: buf.Bytes(), Expiration: exp})
		if err != nil {
			return err
		}

	}

	return nil

}

func (m *Manager) MustWriteSession(w http.ResponseWriter, s *Session) {
	err := m.WriteSession(w, s)
	if err != nil {
		panic(err)
	}
}
