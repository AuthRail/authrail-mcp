package protectedmcp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	authrail "github.com/AuthRail/authrail-sdk-go"
	"golang.org/x/sys/unix"
)

// Records are private recovery state, not proof that a provider effect occurred.
type record struct {
	ResourceID        string                         `json:"resourceId,omitempty"`
	ResourceReference string                         `json:"resourceReference,omitempty"`
	Binding           string                         `json:"binding"`
	Intent            string                         `json:"intent"`
	State             string                         `json:"state"`
	ActionID          string                         `json:"actionId,omitempty"`
	ApprovalID        string                         `json:"approvalId,omitempty"`
	Continuation      *authrail.ApprovalContinuation `json:"continuation,omitempty"`
	Recovery          *authrail.Recovery             `json:"recovery,omitempty"`
}
type store struct {
	root    string
	aead    cipher.AEAD
	lock    *os.File
	stripes [64]sync.Mutex
}

func openStore(root string, key []byte) (*store, error) {
	if len(key) != 32 || !filepath.IsAbs(root) {
		return nil, errors.New("a private absolute state directory and 32-byte journal key are required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("state directory must be private and cannot be a symlink")
	}
	fd, err := unix.Open(filepath.Join(root, ".lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	lock := os.NewFile(uintptr(fd), filepath.Join(root, ".lock"))
	lockInfo, err := lock.Stat()
	if err != nil || !lockInfo.Mode().IsRegular() || lockInfo.Mode().Perm()&0077 != 0 {
		lock.Close()
		return nil, errors.New("journal lock must be a private regular file")
	}
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("this connector state is already in use")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		lock.Close()
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		lock.Close()
		return nil, err
	}
	return &store{root: root, aead: aead, lock: lock}, nil
}
func (s *store) close() error { return s.lock.Close() }
func stateKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *store) acquire(key string) func() {
	sum := sha256.Sum256([]byte(key))
	m := &s.stripes[int(sum[0])%len(s.stripes)]
	m.Lock()
	return m.Unlock
}
func (s *store) load(key string) (*record, error) {
	path := filepath.Join(s.root, key+".state")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil, nil
	}
	body, err := readPrivate(path, 300*1024)
	if err != nil {
		return nil, err
	}

	n := s.aead.NonceSize()
	if len(body) < n {
		return nil, errors.New("invalid recovery state")
	}
	plain, err := s.aead.Open(nil, body[:n], body[n:], []byte(key))
	if err != nil {
		return nil, errors.New("recovery state cannot be authenticated")
	}
	var value record
	if json.Unmarshal(plain, &value) != nil || value.Intent == "" || value.Binding == "" {
		return nil, errors.New("invalid recovery record")
	}
	return &value, nil
}
func (s *store) save(key string, value *record) error {
	plain, err := json.Marshal(value)
	if err != nil || len(plain) > 256*1024 {
		return errors.New("recovery state exceeds its bound")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	encrypted := s.aead.Seal(nonce, nonce, plain, []byte(key))
	file, err := os.CreateTemp(s.root, ".write-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(encrypted)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(s.root, key+".state")); err != nil {
		return err
	}
	directory, err := os.Open(s.root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
