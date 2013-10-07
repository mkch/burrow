package session

import (
	crypto_rand "crypto/rand"
	"math/big"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Objects implementing Session interface can be used  to get and set session value.
type Session interface {
	// Id returns the session id.
	Id() string
	// Value returns the value associated with the session id.
	Value() interface{}
	// SetValue sets the vlaue associated with the session id.
	SetValue(value interface{})
	// AddSessionId adds session id query to the URL. The parameter url is altered
	// and returned.
	AddSessionId(url *url.URL) *url.URL
}

type session struct {
	id           string
	value        interface{}
	ctime, atime time.Time
}

func (s *session) Id() string {
	return s.id
}

func (s *session) Value() interface{} {
	return s.value
}

func (s *session) SetValue(value interface{}) {
	s.value = value
}

func (s *session) CTime() time.Time {
	return s.ctime
}

func (s *session) ATime() time.Time {
	return s.atime
}

func (s *session) AddSessionId(url *url.URL) *url.URL {
	q := url.Query()
	q.Set(SessionIdCookieName, s.id)
	url.RawQuery = q.Encode()
	return url
}

// Object implementing Handler interface can be used to access session value
// while serving http.
//
// Function HTTPHandler(Handler) wraps a Handler to http.Handler.
type Handler interface {
	ServeHTTP(r http.ResponseWriter, w *http.Request, session Session)
}

// The HandlerFunc type is an adapter to allow the use of ordinary functions as Handler.
type HandlerFunc func(r http.ResponseWriter, w *http.Request, session Session)

// ServeHTTP calls f(r, w, session).
func (f HandlerFunc) ServeHTTP(r http.ResponseWriter, w *http.Request, session Session) {
	f(r, w, session)
}

type SessionManager struct {
	sessions map[string]*session
	l        sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{sessions: make(map[string]*session)}
}

// Lookup session by id.
func (s *SessionManager) session(id string) *session {
	s.l.RLock()
	defer func() {
		s.l.RUnlock()
	}()
	return s.sessions[id]
}

// Create new k-v pair
func (s *SessionManager) newSession() (id string, sssn *session) {
	s.l.Lock()
	defer func() {
		s.l.Unlock()
	}()
	for i := 0; i < 99; i++ {
		id = newSessionId()
		if _, exist := s.sessions[id]; !exist {
			now := time.Now()
			sssn = &session{id: id, ctime: now, atime: now}
			s.sessions[id] = sssn
			return
		}
	}
	panic("Can't generate new session id")
}

// InvalidateSession makes a session invalidate. New session will be allocated at
// the next request.
func (s *SessionManager) InvalidateSession(id string) {
	s.l.Lock()
	defer func() {
		s.l.Unlock()
	}()
	delete(s.sessions, id)
}

// Cleanup deletes any sessions that have been idle at least for some duration.
func (s *SessionManager) Cleanup(idle time.Duration) {
	now := time.Now()
	s.l.Lock()
	defer func() {
		s.l.Unlock()
	}()
	for id, session := range s.sessions {
		if now.Sub(session.ATime()) > idle {
			delete(s.sessions, id)
		}
	}
}

// Prepare session things on the request and response.
func (s *SessionManager) prepare(w http.ResponseWriter, r *http.Request) (sessionId string, session *session) {
	// Get session id from query
	sessionId = r.URL.Query().Get(SessionIdCookieName)
	// Get session id from cookie.
	if sessionId == "" {
		if cookie, err := r.Cookie(SessionIdCookieName); err == nil {
			sessionId = cookie.Value
		}
	}
	// Get session from session manager.
	if len(sessionId) == SessionIdLength {
		session = s.session(sessionId)
	}
	// Create new session.
	if session == nil {
		sessionId, session = s.newSession()
		// Construct a cookie
		cookie := &http.Cookie{Name: SessionIdCookieName, Value: sessionId, Path: "/"}
		// Set cookie in response.
		http.SetCookie(w, cookie)
	} else {
		// Touch
		session.atime = time.Now()
	}
	return
}

// Handler wrapps a http.Handler to do session management.
func (s *SessionManager) Handler(handler http.Handler) http.Handler {
	return &handlerHook{manager: s, handler: handler}
}

type handlerHook struct {
	manager *SessionManager
	handler http.Handler
}

func (h *handlerHook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionKey, session := h.manager.prepare(w, r)
	h.handler.ServeHTTP(&responseWriterWithSession{w, sessionKey, session}, r)
}

// HTTPHandlerFunc adapts HandlerFunc to http.Handler
func HTTPHandlerFunc(f HandlerFunc) http.Handler {
	return HTTPHandler(f)
}

// HTTPHandler adapts Handler to http.Handler
func HTTPHandler(h Handler) http.Handler {
	return &handlerWraper{h}
}

type handlerWraper struct {
	Handler
}

func (h *handlerWraper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var session *session
	// "ResponseWriter Hack".
	if s, ok := w.(*responseWriterWithSession); ok {
		session = s.session
	}
	h.Handler.ServeHTTP(w, r, session)
}

// ResponseWriterWrapper is implemented by the http.ResponseWriter object passed
// to Handler.ServeHTTP or HandlerFunc. You can cast the ResponseWriter to this
// interface to get the real ResponseWriter.
//
// For example:
//	func fooHandler(w http.ResponseWriter, r *http.Request, s session.Session) {
//		if wrapper, ok := w.(session.ResponseWriterWrapper); ok {
//			realWriter := wrapper.GetResponseWriter()
//			// Do any thing to realWriter.
//		}
//	}
type ResponseWriterWrapper interface {
	GetResponseWriter() http.ResponseWriter
}

// responseWriterWithSession is a http.ResponseWriter which carries session data.
type responseWriterWithSession struct {
	http.ResponseWriter
	sessionId string
	session   *session
}

func (r *responseWriterWithSession) GetResponseWriter() http.ResponseWriter {
	return r.ResponseWriter
}

// SessionIdCookieName is the cookie name of session id.
const SessionIdCookieName = "__sessionid"

// SessionIdRunes are the characters of which the session id consists.
const SessionIdRunes string = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// SessionIdLength is the length of session id.
const SessionIdLength int = 32

// Generate a new random session id.
func newSessionId() string {
	var bytes [SessionIdLength]byte
	// Use the secure random number as the seed
	if bigSeed, err := crypto_rand.Int(crypto_rand.Reader, big.NewInt(0xFFFFFFFF)); err == nil {
		rand.Seed(bigSeed.Int64())
	} else { // Or use current time.
		rand.Seed(time.Now().UnixNano())
	}
	for i := 0; i < len(bytes); i++ {
		bytes[i] = SessionIdRunes[rand.Int()%len(SessionIdRunes)]
	}
	return string(bytes[:])
}
