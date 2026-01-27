package ssh

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Document stores a document in-memory for uploading to an SFTP server.
type Document struct {
	Name    string `json:"filename"`
	Content string `json:"content"`
}

// sshDial is a function variable to make it possible to unit test SSH code
var sshDial = ssh.Dial

// Connect creates an ssh.Client connected to host:port using the provided username and private key.
func Connect(user, host, key string) (*ssh.Client, error) {
	key = strings.ReplaceAll(key, `\n`, "\n")
	signer, err := ssh.ParsePrivateKey([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	// WARNING: this disables host key checking. Use known_hosts in production!
	hostKeyCallback := ssh.InsecureIgnoreHostKey()

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(host, "22")
	client, err := sshDial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial (addr: %s, user: %s): %w", addr, user, err)
	}

	return client, nil
}

// UploadDocuments writes each document to remoteBaseDir/<name>.xml on the SFTP server.
// It writes to a temp file first and then renames for atomicity.
func UploadDocuments(sftpClient *sftp.Client, data []Document, remoteBaseDir string) error {
	err := sftpClient.MkdirAll(remoteBaseDir)
	if err != nil {
		return fmt.Errorf("mkdirall %s: %w", remoteBaseDir, err)
	}

	for _, d := range data {
		remotePath := path.Join(remoteBaseDir, d.Name)
		tmpPath := remotePath + ".tmp"

		var f *sftp.File
		f, err = sftpClient.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			return fmt.Errorf("open tmp remote file %s: %w", tmpPath, err)
		}

		_, err = io.Copy(f, bytes.NewReader([]byte(d.Content)))
		if err != nil {
			return fmt.Errorf("write tmp file %s: %w", tmpPath, err)
		}

		err = f.Close()
		if err != nil {
			return fmt.Errorf("close tmp file %s: %w", tmpPath, err)
		}

		if err := sftpClient.Rename(tmpPath, remotePath); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", tmpPath, remotePath, err)
		}

		log.Printf("wrote %s (%d bytes)", remotePath, len(d.Content))
	}

	return nil
}
