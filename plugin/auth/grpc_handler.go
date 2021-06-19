package auth

import (
	"bufio"
	"container/list"
	"context"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/DrmagicE/gmqtt/plugin/admin"
)

// List lists all accounts
func (a *Auth) List(ctx context.Context, req *ListAccountsRequest) (resp *ListAccountsResponse, err error) {
	page, pageSize := admin.GetPage(req.Page, req.PageSize)
	offset, n := admin.GetOffsetN(page, pageSize)
	a.mu.RLock()
	defer a.mu.RUnlock()
	resp = &ListAccountsResponse{
		Accounts:   []*Account{},
		TotalCount: 0,
	}
	a.indexer.Iterate(func(elem *list.Element) {
		resp.Accounts = append(resp.Accounts, elem.Value.(*Account))
	}, offset, n)

	resp.TotalCount = uint32(a.indexer.Len())
	return resp, nil
}

// Get gets the account for given username.
// Return NotFound error when account not found.
func (a *Auth) Get(ctx context.Context, req *GetAccountRequest) (resp *GetAccountResponse, err error) {
	if req.Username == "" {
		return nil, admin.ErrInvalidArgument("username", "cannot be empty")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	resp = &GetAccountResponse{}
	if e := a.indexer.GetByID(req.Username); e != nil {
		resp.Account = e.Value.(*Account)
		return resp, nil
	}
	return nil, admin.ErrNotFound
}

// saveFileHandler is the default handler for auth.saveFile, must call after auth.mu is locked
func (a *Auth) saveFileHandler() error {
	tmpfile, err := ioutil.TempFile("./", "gmqtt_password")
	if err != nil {
		return err
	}
	w := bufio.NewWriter(tmpfile)
	// get all accounts
	var accounts []*Account
	a.indexer.Iterate(func(elem *list.Element) {
		accounts = append(accounts, elem.Value.(*Account))
	}, 0, uint(a.indexer.Len()))

	b, err := yaml.Marshal(accounts)
	if err != nil {
		return err
	}

	_, err = w.Write(b)
	if err != nil {
		return err
	}
	err = w.Flush()
	if err != nil {
		return err
	}
	tmpfile.Close()
	// replace the old password file.
	return os.Rename(tmpfile.Name(), a.config.PasswordFile)
}

// Update updates the password for the account.
// Create a new account if the account for the username is not exists.
// Update will persist the account data to the password file.
func (a *Auth) Update(ctx context.Context, req *UpdateAccountRequest) (resp *empty.Empty, err error) {
	if req.Username == "" {
		return nil, admin.ErrInvalidArgument("username", "cannot be empty")
	}
	hashedPassword, err := a.generatePassword(req.Password)
	if err != nil {
		return &empty.Empty{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var oact *Account
	elem := a.indexer.GetByID(req.Username)
	if elem != nil {
		oact = elem.Value.(*Account)
	}
	a.indexer.Set(req.Username, &Account{
		Username: req.Username,
		Password: hashedPassword,
	})
	err = a.saveFile()
	if err != nil {
		// should rollback if failed to persist to file.
		if oact == nil {
			a.indexer.Remove(req.Username)
			return &empty.Empty{}, err
		}
		a.indexer.Set(req.Username, &Account{
			Username: req.Username,
			Password: oact.Password,
		})
	}
	if oact == nil {
		log.Info("new account created", zap.String("username", req.Username))
	} else {
		log.Info("password updated", zap.String("username", req.Username))
	}

	return &empty.Empty{}, err
}

// Delete deletes the account for the username.
func (a *Auth) Delete(ctx context.Context, req *DeleteAccountRequest) (resp *empty.Empty, err error) {
	if req.Username == "" {
		return nil, admin.ErrInvalidArgument("username", "cannot be empty")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	act := a.indexer.GetByID(req.Username)
	if act == nil {
		// fast path
		return &empty.Empty{}, nil
	}
	oact := act.Value
	a.indexer.Remove(req.Username)
	err = a.saveFile()
	if err != nil {
		// should rollback if failed to persist to file
		a.indexer.Set(req.Username, &Account{
			Username: req.Username,
			Password: oact.(*Account).Password,
		})
		return &empty.Empty{}, err
	}
	log.Info("account deleted", zap.String("username", req.Username))
	return &empty.Empty{}, nil
}
