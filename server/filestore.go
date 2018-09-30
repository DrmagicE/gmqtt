package server

import (
	"bufio"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"io"
	"os"
	"sync"

)

var pathSeparator string

func init() {
	gob.Register(&packets.Publish{})
	gob.Register(&packets.Pubrel{})
	if os.IsPathSeparator('\\') {
		pathSeparator = "\\"
	} else {
		pathSeparator = "/"
	}
}

//store sessions & offline msg in files
type FileStore struct {
	Path        string              //dir path
	sync.Mutex                      //gard offlineMsg
	offlineMsg  map[string]*os.File //key by clientId
	sessionFile *os.File
}

func (f *FileStore) newBufioWriterSize(clientId string, size int) (*bufio.Writer, error) {
	var w *os.File
	var err error
	w, err = f.file(clientId)
	if err != nil {
		return nil, err
	}
	if v := bufioWriterPool.Get(); v != nil {
		bw := v.(*bufio.Writer)
		bw.Reset(w)
		return bw, nil
	}
	return bufio.NewWriterSize(w, size), nil
}

//Initialize file for offline msg
func (f *FileStore) file(clientId string) (*os.File, error) {
	var file *os.File
	file = f.offlineMsg[clientId]
	if file == nil {
		h := md5.New()
		h.Write([]byte(clientId))
		md5Str := h.Sum(nil)
		fileName := hex.EncodeToString(md5Str)
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		path := dir + pathSeparator + f.Path + pathSeparator + fileName
		file, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			return nil, err
		}
		f.offlineMsg[clientId] = file
	}
	return file, nil
}

func (f *FileStore) newBufioReaderSize(clientId string, size int) (*bufio.Reader, error) {
	var r *os.File
	var err error
	r, err = f.file(clientId)
	if err != nil {
		return nil, err
	}
	r.Seek(0, 0)
	if v := bufioReaderPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br, nil
	}
	return bufio.NewReaderSize(r, size), nil
}

func (f *FileStore) Open() error {
	var err error
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	path := dir + pathSeparator + f.Path
	dirInfo, err := os.Stat(path)
	if !os.IsNotExist(err) && err != nil {
		return err
	}
	if dirInfo != nil && !dirInfo.IsDir() {
		return errors.New("path is not a dir")
	}
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return err
		}
	}
	filePath := path + pathSeparator + "session.session"
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		f.sessionFile, err = os.Create(filePath)

	} else {
		if fileInfo.IsDir() {
			return errors.New("session file path is a dir")
		}
		f.sessionFile, err = os.OpenFile(filePath, os.O_RDWR | os.O_CREATE, os.ModePerm)
	}
	f.offlineMsg = make(map[string]*os.File)
	return err
}

func (f *FileStore) Close() error {
	var err error
	f.Lock()
	defer f.Unlock()
	for _, v := range f.offlineMsg {
		err = v.Close()
		if err != nil {
			return err
		}
	}
	return f.sessionFile.Close()
}

//Store all offline msg when the session.offlineQueue is reaching the maximum length(set by server.SetMaxOfflineMsg)
func (f *FileStore) PutOfflineMsg(clientId string, packet []packets.Packet) error {
	f.Lock()
	defer f.Unlock()
	bufw, err := f.newBufioWriterSize(clientId, 4096)
	if err != nil {
		return err
	}
	defer putBufioWriter(bufw)
	packetWriter := packets.NewWriter(bufw)
	for _, v := range packet {
		packetWriter.WritePacket(v)
	}
	return packetWriter.Flush()
}

func truncate(file *os.File) error {
	if file != nil {
		err := file.Truncate(0)
		if err != nil {
			return err
		}
		_, err = file.Seek(0, 0)
		if err != nil {
			return err
		}
	}
	return nil
}

//Get offline msg from file when a session-reuse client is connected.
func (f *FileStore) GetOfflineMsg(clientId string) (pp []packets.Packet, err error) {
	f.Lock()
	defer f.Unlock()
	bufr, err := f.newBufioReaderSize(clientId, 4096)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = truncate(f.offlineMsg[clientId])
	}()
	pp = make([]packets.Packet, 0, 200)
	var p packets.Packet
	for err != io.EOF {
		p, err = packets.NewReader(bufr).ReadPacket()
		if err == nil {
			pp = append(pp, p)
		} else {
			if err != io.EOF {
				return nil, err
			}
		}
	}
	return pp, nil
}

//store all sessions when server stop
func (f *FileStore) PutSessions(sp []*SessionPersistence) error {
	return gob.NewEncoder(f.sessionFile).Encode(sp)
}

//get all sessions when server start
func (f *FileStore) GetSessions() ([]*SessionPersistence, error) {
	sp := new([]*SessionPersistence)
	err := gob.NewDecoder(f.sessionFile).Decode(sp)
	if err != nil && err != io.EOF {
		return nil, err
	}
	err = truncate(f.sessionFile)
	if err != nil {
		return nil, err
	}
	return *sp, nil
}
