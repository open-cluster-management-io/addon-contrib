package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches a file for changes and sends the content to a channel.
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	filePath string
	fileName string
}

// New creates a new FileWatcher instance.
func New(filePath string) (*FileWatcher, error) {
	// Create a new fsnotify watcher
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Get the directory of the file to watch
	dirPath := filepath.Dir(filePath)
	// Create the directory if it does not exist
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		log.Printf("Dir %s not exists, creating...", dirPath)
		if err := os.MkdirAll(dirPath, 0777); err != nil {
			fsWatcher.Close()
			return nil, err
		}
	}

	// Add the directory to the watcher
	if err := fsWatcher.Add(dirPath); err != nil {
		fsWatcher.Close()
		return nil, err
	}

	log.Printf("Start listening dir: %s", dirPath)

	// Create a new FileWatcher
	return &FileWatcher{
		watcher:  fsWatcher,
		filePath: filePath,
		fileName: filepath.Base(filePath),
	}, nil
}

// Start starts the file watcher and returns a channel that receives the file content when it changes.
func (fw *FileWatcher) Start(ctx context.Context) <-chan []byte {
	contentChan := make(chan []byte)

	go func() {
		defer func() {
			fw.watcher.Close()
			close(contentChan)
			log.Printf("File watcher closed")
		}()

		for {
			select {
			case event, ok := <-fw.watcher.Events:
				if !ok {
					log.Printf("watcher events not ok")
					return
				}

				eventFileName := filepath.Base(event.Name)

				// Check if the event is for the file we are watching and it is a create or write event
				if eventFileName == fw.fileName &&
					(event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write) {

					// Read the file content
					content, err := os.ReadFile(fw.filePath)
					if err != nil {
						log.Println("Read file err:", err)
					} else {
						log.Printf("Get new content: %s", string(content))
						// Send the content to the channel
						contentChan <- content
					}
				}

			case err, ok := <-fw.watcher.Errors:
				if !ok {
					log.Printf("watcher events not ok")
					return
				}
				log.Println("watcher error:", err)

			case <-ctx.Done():
				log.Println("Receive close signal, closing watcher")
				return
			}
		}
	}()

	return contentChan
}
