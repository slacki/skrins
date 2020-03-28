package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/0xAX/notificator"
	"github.com/atotto/clipboard"
	"github.com/pkg/sftp"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/ssh"
)

var notify *notificator.Notificator

var screensPath string
var remoteHost string
var remoteUser string
var sshKeyPath string
var remotePath string
var baseURL string

func main() {
	exit := make(chan bool)

	flags()

	SFTPClient, err := newSFTPClient()
	if err != nil {
		log.Fatal(err)
	}

	go watchAndUpload(SFTPClient)
	<-exit
}

func flags() {
	flag.StringVar(&screensPath, "p", "", "Path to where screenshots are saved locally")
	flag.StringVar(&remoteHost, "r", "", "Remote host, e.g. example.com:2003 or 43.56.122.31:22")
	flag.StringVar(&remoteUser, "ru", "", "Username on remote host")
	flag.StringVar(&sshKeyPath, "pk", "", "Private key path")
	flag.StringVar(&remotePath, "rp", "", "Path on the remote host")
	flag.StringVar(&baseURL, "url", "", "A base URL that points to given screenshot, e.g https://i.slacki.io/")
	flag.Parse()

	screensPath = strings.TrimRight(screensPath, "/") + "/"
	remotePath = strings.TrimRight(remotePath, "/") + "/"
	baseURL = strings.TrimRight(baseURL, "/") + "/"
}

// watchAndUpload takes anything .png or .jpg and uploads it to the server.
// Files are removed after upload and notification is displayed.
// An URL is copied to the clipboard
func watchAndUpload(client *sftp.Client) {
	for {
		time.Sleep(time.Second)

		fileExtRegexp, _ := regexp.Compile(".*?\\.(\\w+)$")

		fi, err := ioutil.ReadDir(screensPath)
		if err != nil {
			log.Fatal(err)
		}

		for _, f := range fi {
			if f.IsDir() {
				continue
			}

			ext := fileExtRegexp.FindAllStringSubmatch(f.Name(), -1)[0][1]
			if !allowedExtension(ext) {
				continue
			}

			remoteFilename := fmt.Sprintf("%s.%s", uuid.Must(uuid.NewV4(), nil).String(), ext)
			uploadObjectToDestination(client, screensPath+f.Name(), remoteFilename)
			url := baseURL + remoteFilename
			copyToClipboard(url)
			showNotification(url)
			os.Remove(screensPath + f.Name())
		}
	}
}

// showNotification displays a system notification about uploaded screenshot
func showNotification(url string) {
	notify = notificator.New(notificator.Options{
		AppName: "Skrins",
	})
	notify.Push("Screenshot uploaded!", url, "", notificator.UR_NORMAL)
}

func copyToClipboard(s string) {
	clipboard.WriteAll(s)
}

// newSFTPClient creates new sFTP client
func newSFTPClient() (*sftp.Client, error) {
	key, err := ioutil.ReadFile(sshKeyPath)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User: remoteUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", remoteHost, config)
	if err != nil {
		return nil, err
	}
	return sftp.NewClient(client)
}

// allowedExtension determines whether it is allowed to upload a file with that extension
func allowedExtension(ext string) bool {
	allowed := []string{"jpg", "jpeg", "png", "gif", "webm", "mp4"}

	for _, e := range allowed {
		if ext == e {
			return true
		}
	}

	return false
}

// uploadObjectToDestination uploads file to a remote host
func uploadObjectToDestination(client *sftp.Client, src, dest string) {
	// create destination file
	// remotePath is expected to have a trailing slash
	dstFile, err := client.OpenFile(remotePath+dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		log.Fatal(err)
	}
	defer dstFile.Close()

	// open local file
	srcReader, err := os.Open(src)
	if err != nil {
		log.Fatal(err)
	}

	// copy source file to destination file
	bytes, err := io.Copy(dstFile, srcReader)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Total of %d bytes copied\n", bytes)
}