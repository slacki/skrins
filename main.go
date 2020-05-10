package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/0xAX/notificator"
	"github.com/atotto/clipboard"
	"github.com/fsnotify/fsnotify"
	"github.com/lithammer/shortuuid/v3"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var notify *notificator.Notificator
var watcher *fsnotify.Watcher

var screensPath string
var remoteHost string
var remoteUser string
var sshKeyPath string
var remotePath string
var baseURL string

func main() {
	var err error

	flags()

	// creates a new file watcher
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer watcher.Close()

	exit := make(chan bool)

	go watch()

	if err := watcher.Add(screensPath); err != nil {
		panic(err)
	}

	<-exit
}

// flags parses flags
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
func watch() {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				upload()
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				upload()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("error:", err)
		}
	}
}

func upload() {
	fileExtRegexp, _ := regexp.Compile(".*?\\.(\\w+)$")

	fi, err := ioutil.ReadDir(screensPath)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range fi {
		fmt.Println(f.Name())
		if f.IsDir() {
			continue
		}
		fullPath := screensPath + f.Name()

		matches := fileExtRegexp.FindAllStringSubmatch(f.Name(), -1)

		if len(matches) > 0 && len(matches[0]) > 1 {
			ext := matches[0][1]
			if !allowedExtension(ext) {
				continue
			}
			if ext == "mov" {
				log.Println("Detected .mov file, converting to mp4")
				result := ffmpegTranscode(fullPath, screensPath+"out.mp4")
				if result {
					// remove the .mov file if successfully transcoded
					// next pass will upload the file
					os.Remove(fullPath)
					continue
				}
			}

			remoteFilename := fmt.Sprintf("%s.%s", shortuuid.New(), ext)
			err = uploadObjectToDestination(fullPath, remoteFilename)
			if err != nil {
				log.Println(err)
				continue
			}
			url := baseURL + remoteFilename
			copyToClipboard(url)
			showNotification(url)
			os.Remove(fullPath)
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

// copyToClipboard puts a string to clipboards
func copyToClipboard(s string) {
	clipboard.WriteAll(s)
}

// allowedExtension determines whether it is allowed to upload a file with that extension
func allowedExtension(ext string) bool {
	allowed := []string{"jpg", "jpeg", "png", "gif", "webm", "mp4", "mov", "zip", "tar", "tar.gz", "tar.bz2"}

	for _, e := range allowed {
		if ext == e {
			return true
		}
	}

	return false
}

// ffmpegTranscode transcodes a media file.
func ffmpegTranscode(fileIn, fileOut string) bool {
	cmd := exec.Command("/usr/local/bin/ffmpeg", "-i", fileIn, fileOut)
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	err := cmd.Run()

	if err != nil {
		log.Println(err)
		return false
	}
	log.Println("[ffmpeg stderr]", stderr.String())
	log.Println("[ffmpeg stdout]", stdout.String())

	return true
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

// uploadObjectToDestination uploads file to a remote host
func uploadObjectToDestination(src, dest string) error {
	client, err := newSFTPClient()
	if err != nil {
		return err
	}
	defer client.Close()

	// create destination file
	// remotePath is expected to have a trailing slash
	dstFile, err := client.OpenFile(remotePath+dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// open local file
	srcReader, err := os.Open(src)
	if err != nil {
		return err
	}

	// copy source file to destination file
	bytes, err := io.Copy(dstFile, srcReader)
	if err != nil {
		return err
	}

	log.Printf("Total of %d bytes copied\n", bytes)

	return nil
}
