package auth

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/ioutil"
	"os"
	"path"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v2"

	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/plugin/admin"
	"github.com/DrmagicE/gmqtt/server"
)

var _ server.Plugin = (*Auth)(nil)

const Name = "auth"

func init() {
	server.RegisterPlugin(Name, New)
	config.RegisterDefaultPluginConfig(Name, &DefaultConfig)
}

func New(config config.Config) (server.Plugin, error) {
	a := &Auth{
		config:  config.Plugins[Name].(*Config),
		indexer: admin.NewIndexer(),
		pwdDir:  config.ConfigDir,
	}
	a.saveFile = a.saveFileHandler
	return a, nil
}

var log *zap.Logger

// Auth provides the username/password authentication for gmqtt.
// The authentication data is persist in config.PasswordFile.
type Auth struct {
	config *Config
	pwdDir string
	// gard indexer
	mu sync.RWMutex
	// store username/password
	indexer *admin.Indexer
	// saveFile persists the account data to password file.
	saveFile func() error
}

// generatePassword generates the hashed password for the plain password.
func (a *Auth) generatePassword(password string) (hashedPassword string, err error) {
	var h hash.Hash
	switch a.config.Hash {
	case Plain:
		return password, nil
	case MD5:
		h = md5.New()
	case SHA256:
		h = sha256.New()
	case Bcrypt:
		pwd, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		return string(pwd), err
	default:
		// just in case.
		panic("invalid hash type")
	}
	_, err = h.Write([]byte(password))
	if err != nil {
		return "", err
	}
	rs := h.Sum(nil)
	return hex.EncodeToString(rs), nil
}

func (a *Auth) mustEmbedUnimplementedAccountServiceServer() {
	return
}

func (a *Auth) validate(username, password string) (permitted bool, err error) {
	a.mu.RLock()
	elem := a.indexer.GetByID(username)
	a.mu.RUnlock()
	var hashedPassword string
	if elem == nil {
		return false, nil
	}
	ac := elem.Value.(*Account)
	hashedPassword = ac.Password
	var h hash.Hash
	switch a.config.Hash {
	case Plain:
		return hashedPassword == password, nil
	case MD5:
		h = md5.New()
	case SHA256:
		h = sha256.New()
	case Bcrypt:
		return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil, nil
	default:
		// just in case.
		panic("invalid hash type")
	}
	_, err = h.Write([]byte(password))
	if err != nil {
		return false, err
	}
	rs := h.Sum(nil)
	return hashedPassword == hex.EncodeToString(rs), nil
}

var registerAPI = func(service server.Server, a *Auth) error {
	apiRegistrar := service.APIRegistrar()
	RegisterAccountServiceServer(apiRegistrar, a)
	err := apiRegistrar.RegisterHTTPHandler(RegisterAccountServiceHandlerFromEndpoint)
	return err
}

func (a *Auth) Load(service server.Server) error {
	err := registerAPI(service, a)
	log = server.LoggerWithField(zap.String("plugin", Name))

	var pwdFile string
	if path.IsAbs(a.config.PasswordFile) {
		pwdFile = a.config.PasswordFile
	} else {
		pwdFile = path.Join(a.pwdDir, a.config.PasswordFile)
	}
	f, err := os.OpenFile(pwdFile, os.O_CREATE|os.O_RDONLY, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	var acts []*Account
	err = yaml.Unmarshal(b, &acts)
	if err != nil {
		return err
	}
	log.Info("authentication data loaded",
		zap.String("hash", a.config.Hash),
		zap.Int("account_nums", len(acts)),
		zap.String("password_file", pwdFile))

	dup := make(map[string]struct{})
	for _, v := range acts {
		if v.Username == "" {
			return errors.New("detect empty username in password file")
		}
		if _, ok := dup[v.Username]; ok {
			return fmt.Errorf("detect duplicated username in password file: %s", v.Username)
		}
		dup[v.Username] = struct{}{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, v := range acts {
		a.indexer.Set(v.Username, v)
	}
	return nil
}

func (a *Auth) Unload() error {
	return nil
}

func (a *Auth) Name() string {
	return Name
}
